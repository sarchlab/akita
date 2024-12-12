package addresstranslator

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

type transaction struct {
	incomingReqs    []mem.AccessReq
	translationReq  vm.TranslationReq
	translationRsp  vm.TranslationRsp
	translationDone bool
}

type reqToBottom struct {
	reqFromTop  mem.AccessReq
	reqToBottom mem.AccessReq
}

// Comp is an AddressTranslator that forwards the read/write requests with
// the address translated from virtual to physical.
type Comp struct {
	*modeling.TickingComponent
	modeling.MiddlewareHolder

	topPort         modeling.Port
	bottomPort      modeling.Port
	translationPort modeling.Port
	ctrlPort        modeling.Port

	addressToPortMapper mem.AddressToPortMapper
	translationProvider modeling.RemotePort
	log2PageSize        uint64
	deviceID            uint64
	numReqPerCycle      int

	isFlushing bool

	transactions        []*transaction
	inflightReqToBottom []reqToBottom
}

// SetTranslationProvider sets the remote port that can translate addresses.
func (c *Comp) SetTranslationProvider(p modeling.RemotePort) {
	c.translationProvider = p
}

// SetAddressToPortMapper sets the table recording where to find an address.
func (c *Comp) SetAddressToPortMapper(lmf mem.AddressToPortMapper) {
	c.addressToPortMapper = lmf
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

	return madeProgress
}

func (m *middleware) translate() bool {
	item := m.topPort.PeekIncoming()
	if item == nil {
		return false
	}

	req := item.(mem.AccessReq)
	vAddr := req.GetAddress()
	vPageID := m.addrToPageID(vAddr)

	transReq := vm.TranslationReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.translationPort.AsRemote(),
			Dst: m.translationProvider,
		},
		PID:      req.GetPID(),
		VAddr:    vPageID,
		DeviceID: m.deviceID,
	}

	err := m.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	translation := &transaction{
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	m.transactions = append(m.transactions, translation)

	m.traceTransactionStart(req)
	m.traceTranslationStart(req, transReq)

	m.topPort.RetrieveIncoming()

	return true
}

func (m *middleware) parseTranslation() bool {
	rsp := m.translationPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	transRsp := rsp.(vm.TranslationRsp)
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

	m.traceTranslationEnd(transaction.translationReq)
	m.traceMemAccessStart(reqFromTop, translatedReq)

	return true
}

//nolint:funlen,gocyclo
func (m *middleware) respond() bool {
	rsp := m.bottomPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	var (
		reqFromTop       mem.AccessReq
		reqToBottomCombo reqToBottom
		rspToTop         mem.AccessRsp
	)

	reqInBottom := false

	switch rsp := rsp.(type) {
	case mem.DataReadyRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: m.topPort.AsRemote(),
					Dst: reqFromTop.Meta().Src,
				},
				RespondTo: reqFromTop.Meta().ID,
				Data:      rsp.Data,
			}
			rspToTop = drToTop
		}
	case mem.WriteDoneRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRsp{
				MsgMeta: modeling.MsgMeta{
					ID:  id.Generate(),
					Src: m.topPort.AsRemote(),
					Dst: reqFromTop.Meta().Src,
				},
				RespondTo: reqFromTop.Meta().ID,
			}
		}
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := m.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		m.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		m.traceMemAccessEnd(reqToBottomCombo.reqToBottom)
		m.traceTransactionEnd(reqFromTop)
	}

	m.bottomPort.RetrieveIncoming()

	return true
}

func (m *middleware) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case mem.ReadReq:
		return m.createTranslatedReadReq(req, page)
	case mem.WriteReq:
		return m.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (m *middleware) createTranslatedReadReq(
	req mem.ReadReq,
	page vm.Page,
) mem.ReadReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.bottomPort.AsRemote(),
			Dst: m.addressToPortMapper.Find(addr),
		},
		Address:        addr,
		AccessByteSize: req.AccessByteSize,
		PID:            0,
		Info:           req.Info,
	}
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce

	return clone
}

func (m *middleware) createTranslatedWriteReq(
	req mem.WriteReq,
	page vm.Page,
) mem.WriteReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReq{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.bottomPort.AsRemote(),
			Dst: m.addressToPortMapper.Find(addr),
		},
		Data:      req.Data,
		DirtyMask: req.DirtyMask,
		Address:   addr,
		PID:       0,
		Info:      req.Info,
	}
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

	msg := req.(mem.ControlMsg)

	if msg.DiscardTransactions {
		return m.handleFlushReq(msg)
	} else if msg.Restart {
		return m.handleRestartReq(msg)
	}

	panic("never")
}

func (m *middleware) handleFlushReq(
	req mem.ControlMsg,
) bool {
	rsp := mem.ControlMsg{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.ctrlPort.AsRemote(),
			Dst: req.Src,
		},
		NotifyDone: true,
	}

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
	req mem.ControlMsg,
) bool {
	rsp := mem.ControlMsg{
		MsgMeta: modeling.MsgMeta{
			ID:  id.Generate(),
			Src: m.ctrlPort.AsRemote(),
			Dst: req.Src,
		},
		NotifyDone: true,
	}

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

func (m *middleware) traceTransactionStart(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskStart{
			ID:       modeling.ReqInTaskID(req),
			ParentID: modeling.ReqInTaskID(req),
			Kind:     "req_in",
			What:     reflect.TypeOf(req).String(),
		},
		Pos: hooking.HookPosTaskStart,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceTransactionEnd(req mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskEnd{
			ID: modeling.ReqInTaskID(req),
		},
		Pos: hooking.HookPosTaskEnd,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceTranslationStart(
	req mem.AccessReq,
	transReq vm.TranslationReq,
) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskStart{
			ID:       modeling.ReqOutTaskID(transReq),
			ParentID: modeling.ReqInTaskID(req),
			Kind:     "req_out",
			What:     reflect.TypeOf(transReq).String(),
		},
		Pos: hooking.HookPosTaskStart,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceTranslationEnd(
	transReq vm.TranslationReq,
) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskEnd{
			ID: modeling.ReqOutTaskID(transReq),
		},
		Pos: hooking.HookPosTaskEnd,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceMemAccessStart(
	reqFromTop, reqToBottom mem.AccessReq,
) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskStart{
			ID:       modeling.ReqOutTaskID(reqToBottom),
			ParentID: modeling.ReqInTaskID(reqFromTop),
			Kind:     "req_out",
			What:     reflect.TypeOf(reqToBottom).String(),
		},
		Pos: hooking.HookPosTaskStart,
	}

	m.Comp.InvokeHook(ctx)
}

func (m *middleware) traceMemAccessEnd(reqToBottom mem.AccessReq) {
	ctx := hooking.HookCtx{
		Domain: m.Comp,
		Item: hooking.TaskEnd{
			ID: modeling.ReqOutTaskID(reqToBottom),
		},
		Pos: hooking.HookPosTaskEnd,
	}

	m.Comp.InvokeHook(ctx)
}
