package addresstranslator

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"

	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/tracing"
)

type transaction struct {
	incomingReqs    []mem.AccessReq
	translationReq  *vm.TranslationReq
	translationRsp  *vm.TranslationRsp
	translationDone bool
}

type reqToBottom struct {
	reqFromTop  mem.AccessReq
	reqToBottom mem.AccessReq
}

// AddressTranslator is a component that forwards the read/write requests with
// the address translated from virtual to physical.
type AddressTranslator struct {
	*sim.TickingComponent

	topPort         sim.Port
	bottomPort      sim.Port
	translationPort sim.Port
	ctrlPort        sim.Port

	lowModuleFinder     mem.LowModuleFinder
	translationProvider sim.Port
	log2PageSize        uint64
	deviceID            uint64
	numReqPerCycle      int

	isFlushing bool

	transactions        []*transaction
	inflightReqToBottom []reqToBottom

	isWaitingOnGL0InvalidateRsp    bool // GL0InvalidateReq is a request that invalidates the L0 cache. GL0InvalidateRsp is a response to a GL0InvalidateReq.
	currentGL0InvReq               *mem.GL0InvalidateReq
	totalRequestsUponGL0InvArrival int
}

// SetTranslationProvider sets the remote port that can translate addresses.
func (t *AddressTranslator) SetTranslationProvider(p sim.Port) {
	t.translationProvider = p
}

// SetLowModuleFinder sets the table recording where to find an address.
func (t *AddressTranslator) SetLowModuleFinder(lmf mem.LowModuleFinder) {
	t.lowModuleFinder = lmf
}

// Tick updates state at each cycle.
func (t *AddressTranslator) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	if !t.isFlushing {
		madeProgress = t.runPipeline(now)
	}

	madeProgress = t.handleCtrlRequest(now) || madeProgress

	return madeProgress
}

func (t *AddressTranslator) runPipeline(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.respond(now) || madeProgress
	}

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.parseTranslation(now) || madeProgress
	}

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.translate(now) || madeProgress
	}

	madeProgress = t.doGL0Invalidate(now) || madeProgress

	return madeProgress
}

func (t *AddressTranslator) doGL0Invalidate(now sim.VTimeInSec) bool {
	if t.currentGL0InvReq == nil { //if it's nil means no requests to process
		return false
	}

	if t.isWaitingOnGL0InvalidateRsp { //f t.isWaitingOnGL0InvalidateRsp is true, the system is already waiting for a response to a previously sent GL0 invalidate request.
		//The function exits early to avoid re-sending the request.
		return false
	}

	if t.totalRequestsUponGL0InvArrival == 0 { //Check if pending requests exist
		req := mem.GL0InvalidateReqBuilder{}. //Build the GL0 Invalidate Request
							WithPID(t.currentGL0InvReq.PID).
							WithSrc(t.bottomPort).              //through which the request will be sent.
							WithDst(t.lowModuleFinder.Find(0)). // Finds the destination module (e.g., GL0 cache or memory module) using t.lowModuleFinder.Find(0).
							WithSendTime(now).
							Build()

		err := t.bottomPort.Send(req) //sends the request to the designated memory module.
		if err == nil {               // the request was successfully sent:
			t.isWaitingOnGL0InvalidateRsp = true //    Set t.isWaitingOnGL0InvalidateRsp to true, indicating the system is now waiting for a response.
			return true                          //			Return true, indicating progress was made in handling the invalidate request.

		}
	}

	return true
}

// Purpose of translate: Handles translation requests from the top port of the AddressTranslator. It can either process a GL0InvalidateReq or a memory address translation.
func (t *AddressTranslator) translate(now sim.VTimeInSec) bool {
	if t.currentGL0InvReq != nil { //If there is an ongoing GL0InvalidateReq, this method does not proceed with processing new requests, returning false
		return false
	}

	item := t.topPort.Peek() //Retrieves the next item from topPort without removing it.
	if item == nil {
		return false //If no requests are available, return false.
	}

	switch req := item.(type) { //check request type
	case *mem.GL0InvalidateReq: //if its GL0InvalidateReq  handle it
		return t.handleGL0InvalidateReq(now, req)
	}

	req := item.(mem.AccessReq) //Else its memory translation // AccessReq abstracts read and write requests that are sent to the
	// cache modules or memory controllers.
	vAddr := req.GetAddress() // GetAddress returns the address that the request is accessing
	vPageID := t.addrToPageID(vAddr)

	iswrite := true
	switch req := req.(type) {
	case *mem.ReadReq:
		iswrite = false
	case *mem.WriteReq:
		iswrite = true
	default:
		log.Panicf("cannot process request of type %s\n", reflect.TypeOf(req))
	}
	transReq := vm.TranslationReqBuilder{}. //build the request
						WithSendTime(now).
						WithSrc(t.translationPort).
						WithDst(t.translationProvider). //its a port which will translate address like MMU or page Table
						WithPID(req.GetPID()).
						WithVAddr(vPageID).
						WithDeviceID(t.deviceID).
						WithWrite(iswrite).
						Build()
	err := t.translationPort.Send(transReq) //Sends the translation request (transReq) to the translation provider via translationPort.
	if err != nil {                         //if port is busy return false
		return false
	}

	translation := &transaction{ //Creates a new transaction to track the translation
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	t.transactions = append(t.transactions, translation) // append translation

	tracing.TraceReqReceive(req, t)
	tracing.TraceReqInitiate(transReq, t, tracing.MsgIDAtReceiver(req, t))

	t.topPort.Retrieve(now) //remove the processed request from top port

	return true //progress made
}

// T he handleGL0Invalidate function handles the GL0 invalidate request, which typically involves invalidating a certain cache level (L0). It ensures that the current
// invalidate request is processed only if no other GL0 invalidate request is ongoing.
func (t *AddressTranslator) handleGL0InvalidateReq(
	now sim.VTimeInSec,
	req *mem.GL0InvalidateReq,
) bool {
	if t.currentGL0InvReq != nil { // Check if there is already an ongoing GL0 invalidate request
		return false //If there is an ongoing request (i.e., currentGL0InvReq is not nil), it returns false, indicating that the new request cannot be processed at the moment.
	}

	t.currentGL0InvReq = req //If no request is currently being processed, the incoming invalidate request (req) is assigned to currentGL0InvReq. This indicates that this request is now the one being processed.
	t.totalRequestsUponGL0InvArrival =
		len(t.transactions) + len(t.inflightReqToBottom) //The variable totalRequestsUponGL0InvArrival is updated to track the total number of requests that are either currently being processed (t.transactions) or are inflight (t.inflightReqToBottom).
	t.topPort.Retrieve(now) //e function retrieves the next request from the topPort. The Retrieve function might be responsible for getting the next available request from a queue or buffer.

	return true //After processing the request, the function returns true, indicating that the GL0 invalidate request has been successfully handled.
}

/*
Purpose parseTranslation: The function handles parsing the translation response, processes it, and sends the translated request to the next port in the system.
It manages memory or address translation requests and responses.
*/
func (t *AddressTranslator) parseTranslation(now sim.VTimeInSec) bool {
	rsp := t.translationPort.Peek() //This line checks the translationPort to see if there is a pending translation response. The Peek method retrieves the item from the port without removing it from the queue.
	if rsp == nil {                 //No pending response
		return false
	}

	transRsp := rsp.(*vm.TranslationRsp) //This line casts the rsp variable to a *vm.TranslationRsp type.Since rsp could be of any type, type assertion is
	//used to ensure that it is specifically a TranslationRsp. If the type assertion fails (i.e., the response is not of the expected type), it would result in a runtime error.

	transaction := t.findTranslationByReqID(transRsp.RespondTo) //The function looks up the transaction associated with the translation response.
	//The RespondTo field in the TranslationRsp holds the ID of the request that this response corresponds to.

	if transaction == nil {
		t.translationPort.Retrieve(now)
		return true
	}
	/*
	 	If no matching transaction is found, the function retrieves the next item from the translationPort and returns true, indicating that the operation has been processed (even though no translation was found).
	    This could be a situation where the response was received before the corresponding request was found, possibly due to out-of-order message delivery.
	*/

	//: Set the Translation Response to the Transaction
	transaction.translationRsp = transRsp     //    If a matching transaction is found, the translationRsp field of the transaction is updated with the newly received translation response.
	transaction.translationDone = true        //    The translationDone field is set to true, indicating that the translation process for this transaction is now complete.
	reqFromTop := transaction.incomingReqs[0] //Create the Translated Request.The first request in the incomingReqs list of the transaction is extracted (likely the original memory access request that triggered the translation).

	translatedReq := t.createTranslatedReq(
		reqFromTop,
		transaction.translationRsp.Page)

	translatedReq.Meta().SendTime = now

	err := t.bottomPort.Send(translatedReq) //The translatedReq is sent via bottomPort. The Send function returns an error if the request could not be sent, and if there is an error, the function would return false
	if err != nil {
		return false
	}

	/*The translated request is added to the inflightReqToBottom list, which tracks requests that have been sent but not yet processed by the bottom module.
	This is necessary for keeping track of the state of outstanding requests.*/
	t.inflightReqToBottom = append(t.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	transaction.incomingReqs = transaction.incomingReqs[1:] //The processed request (reqFromTop) is removed from the incomingReqs list of the transaction.
	if len(transaction.incomingReqs) == 0 {                 //If there are no more requests in the incomingReqs list, it indicates that all requests in this transaction have been processed.
		//The function then removes the transaction from the system using removeExistingTranslation.
		t.removeExistingTranslation(transaction)
	}

	t.translationPort.Retrieve(now) //Retrieve the Next Request from the Translation Port

	tracing.TraceReqFinalize(transaction.translationReq, t)
	tracing.TraceReqInitiate(translatedReq, t,
		tracing.MsgIDAtReceiver(reqFromTop, t))

	return true
}

//nolint:funlen,gocyclo
func (t *AddressTranslator) respond(now sim.VTimeInSec) bool {
	rsp := t.bottomPort.Peek()
	if rsp == nil {
		return false
	}

	reqInBottom := false
	gl0InvalidateRsp := false

	var reqFromTop mem.AccessReq
	var reqToBottomCombo reqToBottom
	var rspToTop mem.AccessRsp
	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		reqInBottom = t.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = t.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRspBuilder{}.
				WithSendTime(now).
				WithSrc(t.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				WithData(rsp.Data).
				Build()
			rspToTop = drToTop
		}
	case *mem.WriteDoneRsp:
		reqInBottom = t.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = t.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSendTime(now).
				WithSrc(t.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				Build()
		}
	case *mem.GL0InvalidateRsp:
		gl0InvalidateReq := t.currentGL0InvReq
		if gl0InvalidateReq == nil {
			log.Panicf("Cannot have rsp without req")
		}
		rspToTop = mem.GL0InvalidateRspBuilder{}.
			WithSendTime(now).
			WithSrc(t.topPort).
			WithDst(gl0InvalidateReq.Src).
			WithRspTo(gl0InvalidateReq.Meta().ID).
			Build()
		gl0InvalidateRsp = true
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := t.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		t.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, t)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, t)
	}

	if gl0InvalidateRsp {
		err := t.topPort.Send(rspToTop)
		if err != nil {
			return false
		}
		t.currentGL0InvReq = nil
		t.isWaitingOnGL0InvalidateRsp = false
		if t.totalRequestsUponGL0InvArrival != 0 {
			log.Panicf("Something went wrong \n")
		}
	}

	if t.currentGL0InvReq != nil {
		t.totalRequestsUponGL0InvArrival--

		if t.totalRequestsUponGL0InvArrival < 0 {
			log.Panicf("Not possible")
		}
	}

	t.bottomPort.Retrieve(now)
	return true
}

func (t *AddressTranslator) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return t.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return t.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (t *AddressTranslator) createTranslatedReadReq(
	req *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := req.Address % (1 << t.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(t.bottomPort).
		WithDst(t.lowModuleFinder.Find(addr)).
		WithAddress(addr).
		WithByteSize(req.AccessByteSize).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (t *AddressTranslator) createTranslatedWriteReq(
	req *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := req.Address % (1 << t.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(t.bottomPort).
		WithDst(t.lowModuleFinder.Find(addr)).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (t *AddressTranslator) addrToPageID(addr uint64) uint64 { //givers starting address of page
	return (addr >> t.log2PageSize) << t.log2PageSize
}

func (t *AddressTranslator) findTranslationByReqID(id string) *transaction {
	for _, t := range t.transactions {
		if t.translationReq.ID == id {
			return t
		}
	}
	return nil
}

func (t *AddressTranslator) removeExistingTranslation(trans *transaction) {
	for i, tr := range t.transactions {
		if tr == trans {
			t.transactions = append(t.transactions[:i], t.transactions[i+1:]...)
			return
		}
	}
	panic("translation not found")
}

func (t *AddressTranslator) isReqInBottomByID(id string) bool {
	for _, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return true
		}
	}
	return false
}

func (t *AddressTranslator) findReqToBottomByID(id string) reqToBottom {
	for _, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func (t *AddressTranslator) removeReqToBottomByID(id string) {
	for i, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			t.inflightReqToBottom = append(
				t.inflightReqToBottom[:i],
				t.inflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

func (t *AddressTranslator) handleCtrlRequest(now sim.VTimeInSec) bool {
	req := t.ctrlPort.Peek()
	if req == nil {
		return false
	}

	msg := req.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return t.handleFlushReq(now, msg)
	} else if msg.Restart {
		return t.handleRestartReq(now, msg)
	}

	panic("never")
}

func (t *AddressTranslator) handleFlushReq(
	now sim.VTimeInSec,
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(t.ctrlPort).
		WithDst(req.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := t.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	t.ctrlPort.Retrieve(now)

	t.transactions = nil
	t.inflightReqToBottom = nil
	t.isFlushing = true

	return true
}

func (t *AddressTranslator) handleRestartReq(
	now sim.VTimeInSec,
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(t.ctrlPort).
		WithDst(req.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := t.ctrlPort.Send(rsp)

	if err != nil {
		return false
	}

	for t.topPort.Retrieve(now) != nil {
	}

	for t.bottomPort.Retrieve(now) != nil {
	}

	for t.translationPort.Retrieve(now) != nil {
	}

	t.isFlushing = false

	t.ctrlPort.Retrieve(now)

	return true
}
