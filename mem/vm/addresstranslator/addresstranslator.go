package addresstranslator

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/tracing"
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

// Comp is an AddressTranslator that forwards the read/write requests with
// the address translated from virtual to physical.
type Comp struct {
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

	isWaitingOnGL0InvalidateRsp    bool
	currentGL0InvReq               *mem.GL0InvalidateReq
	totalRequestsUponGL0InvArrival int
}

// SetTranslationProvider sets the remote port that can translate addresses.
func (c *Comp) SetTranslationProvider(p sim.Port) {
	c.translationProvider = p
}

// SetLowModuleFinder sets the table recording where to find an address.
func (c *Comp) SetLowModuleFinder(lmf mem.LowModuleFinder) {
	c.lowModuleFinder = lmf
}

// Tick updates state at each cycle.
func (c *Comp) Tick() bool {
	madeProgress := false

	if !c.isFlushing {
		madeProgress = c.runPipeline()
	}

	madeProgress = c.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (c *Comp) runPipeline() bool {
	madeProgress := false

	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.respond() || madeProgress
	}

	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.parseTranslation() || madeProgress
	}

	for i := 0; i < c.numReqPerCycle; i++ {
		madeProgress = c.translate() || madeProgress
	}

	madeProgress = c.doGL0Invalidate() || madeProgress

	return madeProgress
}

func (c *Comp) doGL0Invalidate() bool {
	if c.currentGL0InvReq == nil {
		return false
	}

	if c.isWaitingOnGL0InvalidateRsp {
		return false
	}

	if c.totalRequestsUponGL0InvArrival == 0 {
		req := mem.GL0InvalidateReqBuilder{}.
			WithPID(c.currentGL0InvReq.PID).
			WithSrc(c.bottomPort).
			WithDst(c.lowModuleFinder.Find(0)).
			Build()

		err := c.bottomPort.Send(req)
		if err == nil {
			c.isWaitingOnGL0InvalidateRsp = true
			return true
		}
	}

	return true
}

func (c *Comp) translate() bool {
	if c.currentGL0InvReq != nil {
		return false
	}

	item := c.topPort.PeekIncoming()
	if item == nil {
		return false
	}

	switch req := item.(type) {
	case *mem.GL0InvalidateReq:
		return c.handleGL0InvalidateReq(req)
	}

	req := item.(mem.AccessReq)
	vAddr := req.GetAddress()
	vPageID := c.addrToPageID(vAddr)

	transReq := vm.TranslationReqBuilder{}.
		WithSrc(c.translationPort).
		WithDst(c.translationProvider).
		WithPID(req.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(c.deviceID).
		Build()
	err := c.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	translation := &transaction{
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	c.transactions = append(c.transactions, translation)

	tracing.TraceReqReceive(req, c)
	tracing.TraceReqInitiate(transReq, c, tracing.MsgIDAtReceiver(req, c))

	c.topPort.RetrieveIncoming()

	return true
}

func (c *Comp) handleGL0InvalidateReq(
	req *mem.GL0InvalidateReq,
) bool {
	if c.currentGL0InvReq != nil {
		return false
	}

	c.currentGL0InvReq = req
	c.totalRequestsUponGL0InvArrival =
		len(c.transactions) + len(c.inflightReqToBottom)
	c.topPort.RetrieveIncoming()

	return true
}

func (c *Comp) parseTranslation() bool {
	rsp := c.translationPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	transRsp := rsp.(*vm.TranslationRsp)
	transaction := c.findTranslationByReqID(transRsp.RespondTo)
	if transaction == nil {
		c.translationPort.RetrieveIncoming()
		return true
	}

	transaction.translationRsp = transRsp
	transaction.translationDone = true
	reqFromTop := transaction.incomingReqs[0]
	translatedReq := c.createTranslatedReq(
		reqFromTop,
		transaction.translationRsp.Page)
	err := c.bottomPort.Send(translatedReq)
	if err != nil {
		return false
	}

	c.inflightReqToBottom = append(c.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	transaction.incomingReqs = transaction.incomingReqs[1:]
	if len(transaction.incomingReqs) == 0 {
		c.removeExistingTranslation(transaction)
	}

	c.translationPort.RetrieveIncoming()

	tracing.TraceReqFinalize(transaction.translationReq, c)
	tracing.TraceReqInitiate(translatedReq, c,
		tracing.MsgIDAtReceiver(reqFromTop, c))

	return true
}

//nolint:funlen,gocyclo
func (c *Comp) respond() bool {
	rsp := c.bottomPort.PeekIncoming()
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
		reqInBottom = c.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = c.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRspBuilder{}.
				WithSrc(c.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				WithData(rsp.Data).
				Build()
			rspToTop = drToTop
		}
	case *mem.WriteDoneRsp:
		reqInBottom = c.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = c.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSrc(c.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				Build()
		}
	case *mem.GL0InvalidateRsp:
		gl0InvalidateReq := c.currentGL0InvReq
		if gl0InvalidateReq == nil {
			log.Panicf("Cannot have rsp without req")
		}
		rspToTop = mem.GL0InvalidateRspBuilder{}.
			WithSrc(c.topPort).
			WithDst(gl0InvalidateReq.Src).
			WithRspTo(gl0InvalidateReq.Meta().ID).
			Build()
		gl0InvalidateRsp = true
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := c.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		c.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, c)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, c)
	}

	if gl0InvalidateRsp {
		err := c.topPort.Send(rspToTop)
		if err != nil {
			return false
		}
		c.currentGL0InvReq = nil
		c.isWaitingOnGL0InvalidateRsp = false
		if c.totalRequestsUponGL0InvArrival != 0 {
			log.Panicf("Something went wrong \n")
		}
	}

	if c.currentGL0InvReq != nil {
		c.totalRequestsUponGL0InvArrival--

		if c.totalRequestsUponGL0InvArrival < 0 {
			log.Panicf("Not possible")
		}
	}

	c.bottomPort.RetrieveIncoming()
	return true
}

func (c *Comp) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return c.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return c.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (c *Comp) createTranslatedReadReq(
	req *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := req.Address % (1 << c.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(c.bottomPort).
		WithDst(c.lowModuleFinder.Find(addr)).
		WithAddress(addr).
		WithByteSize(req.AccessByteSize).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (c *Comp) createTranslatedWriteReq(
	req *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := req.Address % (1 << c.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(c.bottomPort).
		WithDst(c.lowModuleFinder.Find(addr)).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (c *Comp) addrToPageID(addr uint64) uint64 {
	return (addr >> c.log2PageSize) << c.log2PageSize
}

func (c *Comp) findTranslationByReqID(id string) *transaction {
	for _, t := range c.transactions {
		if t.translationReq.ID == id {
			return t
		}
	}
	return nil
}

func (c *Comp) removeExistingTranslation(trans *transaction) {
	for i, tr := range c.transactions {
		if tr == trans {
			c.transactions = append(c.transactions[:i], c.transactions[i+1:]...)
			return
		}
	}
	panic("translation not found")
}

func (c *Comp) isReqInBottomByID(id string) bool {
	for _, r := range c.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return true
		}
	}
	return false
}

func (c *Comp) findReqToBottomByID(id string) reqToBottom {
	for _, r := range c.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func (c *Comp) removeReqToBottomByID(id string) {
	for i, r := range c.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			c.inflightReqToBottom = append(
				c.inflightReqToBottom[:i],
				c.inflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

func (c *Comp) handleCtrlRequest() bool {
	req := c.ctrlPort.PeekIncoming()
	if req == nil {
		return false
	}

	msg := req.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return c.handleFlushReq(msg)
	} else if msg.Restart {
		return c.handleRestartReq(msg)
	}

	panic("never")
}

func (c *Comp) handleFlushReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(c.ctrlPort).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := c.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	c.ctrlPort.RetrieveIncoming()

	c.transactions = nil
	c.inflightReqToBottom = nil
	c.isFlushing = true

	return true
}

func (c *Comp) handleRestartReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(c.ctrlPort).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := c.ctrlPort.Send(rsp)

	if err != nil {
		return false
	}

	for c.topPort.RetrieveIncoming() != nil {
	}

	for c.bottomPort.RetrieveIncoming() != nil {
	}

	for c.translationPort.RetrieveIncoming() != nil {
	}

	c.isFlushing = false

	c.ctrlPort.RetrieveIncoming()

	return true
}
