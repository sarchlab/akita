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
	sim.MiddlewareHolder

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

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates state at each cycle.
func (m *middleware) Tick() bool {
	madeProgress := false

	if !m.isFlushing {
		madeProgress = m.runPipeline()
	}

	madeProgress = m.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respond() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseTranslation() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.translate() || madeProgress
	}

	madeProgress = m.doGL0Invalidate() || madeProgress

	return madeProgress
}

func (m *middleware) doGL0Invalidate() bool {
	if m.currentGL0InvReq == nil {
		return false
	}

	if m.isWaitingOnGL0InvalidateRsp {
		return false
	}

	if m.totalRequestsUponGL0InvArrival == 0 {
		req := mem.GL0InvalidateReqBuilder{}.
			WithPID(m.currentGL0InvReq.PID).
			WithSrc(m.bottomPort).
			WithDst(m.lowModuleFinder.Find(0)).
			Build()

		err := m.bottomPort.Send(req)
		if err == nil {
			m.isWaitingOnGL0InvalidateRsp = true
			return true
		}
	}

	return true
}

func (m *middleware) translate() bool {
	if m.currentGL0InvReq != nil {
		return false
	}

	item := m.topPort.PeekIncoming()
	if item == nil {
		return false
	}

	switch req := item.(type) {
	case *mem.GL0InvalidateReq:
		return m.handleGL0InvalidateReq(req)
	}

	req := item.(mem.AccessReq)
	vAddr := req.GetAddress()
	vPageID := m.addrToPageID(vAddr)

	transReq := vm.TranslationReqBuilder{}.
		WithSrc(m.translationPort).
		WithDst(m.translationProvider).
		WithPID(req.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(m.deviceID).
		Build()
	err := m.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	translation := &transaction{
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	m.transactions = append(m.transactions, translation)

	tracing.TraceReqReceive(req, m.Comp)
	tracing.TraceReqInitiate(transReq, m.Comp, tracing.MsgIDAtReceiver(req, m.Comp))

	m.topPort.RetrieveIncoming()

	return true
}

func (m *middleware) handleGL0InvalidateReq(
	req *mem.GL0InvalidateReq,
) bool {
	if m.currentGL0InvReq != nil {
		return false
	}

	m.currentGL0InvReq = req
	m.totalRequestsUponGL0InvArrival =
		len(m.transactions) + len(m.inflightReqToBottom)
	m.topPort.RetrieveIncoming()

	return true
}

func (m *middleware) parseTranslation() bool {
	rsp := m.translationPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	transRsp := rsp.(*vm.TranslationRsp)
	transaction := m.findTranslationByReqID(transRsp.RespondTo)
	if transaction == nil {
		m.translationPort.RetrieveIncoming()
		return true
	}

	transaction.translationRsp = transRsp
	transaction.translationDone = true
	reqFromTop := transaction.incomingReqs[0]
	translatedReq := m.createTranslatedReq(
		reqFromTop,
		transaction.translationRsp.Page)
	err := m.bottomPort.Send(translatedReq)
	if err != nil {
		return false
	}

	m.inflightReqToBottom = append(m.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	transaction.incomingReqs = transaction.incomingReqs[1:]
	if len(transaction.incomingReqs) == 0 {
		m.removeExistingTranslation(transaction)
	}

	m.translationPort.RetrieveIncoming()

	tracing.TraceReqFinalize(transaction.translationReq, m.Comp)
	tracing.TraceReqInitiate(translatedReq, m.Comp,
		tracing.MsgIDAtReceiver(reqFromTop, m.Comp))

	return true
}

//nolint:funlen,gocyclo
func (m *middleware) respond() bool {
	rsp := m.bottomPort.PeekIncoming()
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
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRspBuilder{}.
				WithSrc(m.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				WithData(rsp.Data).
				Build()
			rspToTop = drToTop
		}
	case *mem.WriteDoneRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSrc(m.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				Build()
		}
	case *mem.GL0InvalidateRsp:
		gl0InvalidateReq := m.currentGL0InvReq
		if gl0InvalidateReq == nil {
			log.Panicf("Cannot have rsp without req")
		}
		rspToTop = mem.GL0InvalidateRspBuilder{}.
			WithSrc(m.topPort).
			WithDst(gl0InvalidateReq.Src).
			WithRspTo(gl0InvalidateReq.Meta().ID).
			Build()
		gl0InvalidateRsp = true
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := m.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		m.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, m.Comp)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, m.Comp)
	}

	if gl0InvalidateRsp {
		err := m.topPort.Send(rspToTop)
		if err != nil {
			return false
		}
		m.currentGL0InvReq = nil
		m.isWaitingOnGL0InvalidateRsp = false
		if m.totalRequestsUponGL0InvArrival != 0 {
			log.Panicf("Something went wrong \n")
		}
	}

	if m.currentGL0InvReq != nil {
		m.totalRequestsUponGL0InvArrival--

		if m.totalRequestsUponGL0InvArrival < 0 {
			log.Panicf("Not possible")
		}
	}

	m.bottomPort.RetrieveIncoming()
	return true
}

func (m *middleware) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return m.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return m.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (m *middleware) createTranslatedReadReq(
	req *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(m.bottomPort).
		WithDst(m.lowModuleFinder.Find(addr)).
		WithAddress(addr).
		WithByteSize(req.AccessByteSize).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (m *middleware) createTranslatedWriteReq(
	req *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(m.bottomPort).
		WithDst(m.lowModuleFinder.Find(addr)).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (m *middleware) addrToPageID(addr uint64) uint64 {
	return (addr >> m.log2PageSize) << m.log2PageSize
}

func (m *middleware) findTranslationByReqID(id string) *transaction {
	for _, t := range m.transactions {
		if t.translationReq.ID == id {
			return t
		}
	}
	return nil
}

func (m *middleware) removeExistingTranslation(trans *transaction) {
	for i, tr := range m.transactions {
		if tr == trans {
			m.transactions = append(m.transactions[:i], m.transactions[i+1:]...)
			return
		}
	}
	panic("translation not found")
}

func (m *middleware) isReqInBottomByID(id string) bool {
	for _, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return true
		}
	}
	return false
}

func (m *middleware) findReqToBottomByID(id string) reqToBottom {
	for _, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func (m *middleware) removeReqToBottomByID(id string) {
	for i, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			m.inflightReqToBottom = append(
				m.inflightReqToBottom[:i],
				m.inflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

func (m *middleware) handleCtrlRequest() bool {
	req := m.ctrlPort.PeekIncoming()
	if req == nil {
		return false
	}

	msg := req.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return m.handleFlushReq(msg)
	} else if msg.Restart {
		return m.handleRestartReq(msg)
	}

	panic("never")
}

func (m *middleware) handleFlushReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.ctrlPort.RetrieveIncoming()

	m.transactions = nil
	m.inflightReqToBottom = nil
	m.isFlushing = true

	return true
}

func (m *middleware) handleRestartReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := m.ctrlPort.Send(rsp)

	if err != nil {
		return false
	}

	for m.topPort.RetrieveIncoming() != nil {
	}

	for m.bottomPort.RetrieveIncoming() != nil {
	}

	for m.translationPort.RetrieveIncoming() != nil {
	}

	m.isFlushing = false

	m.ctrlPort.RetrieveIncoming()

	return true
}
