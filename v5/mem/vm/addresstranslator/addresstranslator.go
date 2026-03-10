package addresstranslator

import (
	"io"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Spec contains immutable configuration for the AddressTranslator.
type Spec struct {
	Log2PageSize   uint64 `json:"log2_page_size"`
	DeviceID       uint64 `json:"device_id"`
	NumReqPerCycle int    `json:"num_req_per_cycle"`
}

// incomingReqState is a serializable representation of an incoming *sim.GenericMsg.
type incomingReqState struct {
	ID    string         `json:"id"`
	Src   sim.RemotePort `json:"src"`
	Dst   sim.RemotePort `json:"dst"`
	RspTo string         `json:"rsp_to"`
}

// transactionState is a serializable representation of a runtime transaction.
type transactionState struct {
	IncomingReqs      []incomingReqState `json:"incoming_reqs"`
	TranslationReqID  string             `json:"translation_req_id"`
	TranslationReqSrc sim.RemotePort     `json:"translation_req_src"`
	TranslationReqDst sim.RemotePort     `json:"translation_req_dst"`
	TranslationDone   bool               `json:"translation_done"`
}

// reqToBottomState is a serializable representation of a runtime reqToBottom.
type reqToBottomState struct {
	ReqFromTopID  string         `json:"req_from_top_id"`
	ReqFromTopSrc sim.RemotePort `json:"req_from_top_src"`
	ReqFromTopDst sim.RemotePort `json:"req_from_top_dst"`
	ReqToBottomID  string         `json:"req_to_bottom_id"`
	ReqToBottomSrc sim.RemotePort `json:"req_to_bottom_src"`
	ReqToBottomDst sim.RemotePort `json:"req_to_bottom_dst"`
}

// State contains mutable runtime data for the AddressTranslator.
type State struct {
	IsFlushing          bool               `json:"is_flushing"`
	Transactions        []transactionState `json:"transactions"`
	InflightReqToBottom []reqToBottomState `json:"inflight_req_to_bottom"`
}

type transaction struct {
	incomingReqs    []*sim.GenericMsg
	translationReq  *sim.GenericMsg // payload: *vm.TranslationReqPayload
	translationRsp  *sim.GenericMsg // payload: *vm.TranslationRspPayload
	translationDone bool
}

type reqToBottom struct {
	reqFromTop  *sim.GenericMsg
	reqToBottom *sim.GenericMsg
}

// Comp is an AddressTranslator that forwards the read/write requests with
// the address translated from virtual to physical.
type Comp struct {
	*modeling.Component[Spec, State]

	topPort         sim.Port
	bottomPort      sim.Port
	translationPort sim.Port
	ctrlPort        sim.Port

	memoryPortMapper      mem.AddressToPortMapper
	translationPortMapper mem.AddressToPortMapper

	isFlushing bool

	transactions        []*transaction
	inflightReqToBottom []reqToBottom
}

// GetState converts runtime mutable data into a serializable State.
func (c *Comp) GetState() State {
	state := State{
		IsFlushing: c.isFlushing,
	}

	for _, t := range c.transactions {
		ts := transactionState{
			TranslationDone: t.translationDone,
		}

		for _, req := range t.incomingReqs {
			ts.IncomingReqs = append(ts.IncomingReqs, incomingReqState{
				ID:    req.ID,
				Src:   req.Src,
				Dst:   req.Dst,
				RspTo: req.RspTo,
			})
		}

		if t.translationReq != nil {
			ts.TranslationReqID = t.translationReq.ID
			ts.TranslationReqSrc = t.translationReq.Src
			ts.TranslationReqDst = t.translationReq.Dst
		}

		state.Transactions = append(state.Transactions, ts)
	}

	for _, r := range c.inflightReqToBottom {
		state.InflightReqToBottom = append(state.InflightReqToBottom,
			reqToBottomState{
				ReqFromTopID:   r.reqFromTop.ID,
				ReqFromTopSrc:  r.reqFromTop.Src,
				ReqFromTopDst:  r.reqFromTop.Dst,
				ReqToBottomID:  r.reqToBottom.ID,
				ReqToBottomSrc: r.reqToBottom.Src,
				ReqToBottomDst: r.reqToBottom.Dst,
			})
	}

	c.Component.SetState(state)

	return state
}

// SetState restores runtime mutable data from a serializable State.
func (c *Comp) SetState(state State) {
	c.Component.SetState(state)

	c.isFlushing = state.IsFlushing

	c.transactions = nil
	for _, ts := range state.Transactions {
		t := &transaction{
			translationDone: ts.TranslationDone,
		}

		for _, reqState := range ts.IncomingReqs {
			t.incomingReqs = append(t.incomingReqs, &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:    reqState.ID,
					Src:   reqState.Src,
					Dst:   reqState.Dst,
					RspTo: reqState.RspTo,
				},
			})
		}

		if ts.TranslationReqID != "" {
			t.translationReq = &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  ts.TranslationReqID,
					Src: ts.TranslationReqSrc,
					Dst: ts.TranslationReqDst,
				},
			}
		}

		c.transactions = append(c.transactions, t)
	}

	c.inflightReqToBottom = nil
	for _, rs := range state.InflightReqToBottom {
		c.inflightReqToBottom = append(c.inflightReqToBottom, reqToBottom{
			reqFromTop: &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  rs.ReqFromTopID,
					Src: rs.ReqFromTopSrc,
					Dst: rs.ReqFromTopDst,
				},
			},
			reqToBottom: &sim.GenericMsg{
				MsgMeta: sim.MsgMeta{
					ID:  rs.ReqToBottomID,
					Src: rs.ReqToBottomSrc,
					Dst: rs.ReqToBottomDst,
				},
			},
		})
	}
}

// SaveState syncs runtime data to state, then delegates to Component.SaveState.
func (c *Comp) SaveState(w io.Writer) error {
	c.GetState()
	return c.Component.SaveState(w)
}

// LoadState loads state from the reader, then restores runtime fields.
func (c *Comp) LoadState(r io.Reader) error {
	if err := c.Component.LoadState(r); err != nil {
		return err
	}

	c.SetState(c.Component.GetState())

	return nil
}

type middleware struct {
	*Comp
}

// Tick updates state at each cycle.
func (m *middleware) Tick() bool {
	madeProgress := false

	if !m.isFlushing {
		madeProgress = m.runPipeline()
	} else {
		for i := 0; i < m.GetSpec().NumReqPerCycle; i++ {
			madeProgress = m.parseTranslation() || madeProgress
		}
	}

	madeProgress = m.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	spec := m.GetSpec()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respond() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseTranslation() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.translate() || madeProgress
	}

	return madeProgress
}

func (m *middleware) translate() bool {
	itemI := m.topPort.PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(*sim.GenericMsg)
	payload := item.Payload.(mem.AccessReqPayload)
	vAddr := payload.GetAddress()
	vPageID := m.addrToPageID(vAddr)

	transReq := vm.TranslationReqBuilder{}.
		WithSrc(m.translationPort.AsRemote()).
		WithDst(m.translationPortMapper.Find(vAddr)).
		WithPID(payload.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(m.GetSpec().DeviceID).
		Build()

	err := m.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	trans := &transaction{
		incomingReqs:   []*sim.GenericMsg{item},
		translationReq: transReq,
	}
	m.transactions = append(m.transactions, trans)

	tracing.TraceReqReceive(item, m.Comp)
	tracing.TraceReqInitiate(
		transReq,
		m.Comp,
		tracing.MsgIDAtReceiver(item, m.Comp),
	)

	m.topPort.RetrieveIncoming()

	return true
}

func (m *middleware) parseTranslation() bool {
	rspI := m.translationPort.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(*sim.GenericMsg)
	trans := m.findTranslationByReqID(rsp.RspTo)

	if trans == nil {
		m.translationPort.RetrieveIncoming()
		return true
	}

	trans.translationRsp = rsp
	trans.translationDone = true

	rspPayload := sim.MsgPayload[vm.TranslationRspPayload](rsp)

	reqFromTop := trans.incomingReqs[0]
	translatedReq := m.createTranslatedReq(reqFromTop, rspPayload.Page)

	err := m.bottomPort.Send(translatedReq)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(translatedReq, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	m.inflightReqToBottom = append(m.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	trans.incomingReqs = trans.incomingReqs[1:]

	if len(trans.incomingReqs) == 0 {
		m.removeExistingTranslation(trans)
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
		tracing.MilestoneKindTranslation,
		"translation",
		m.Comp.Name(),
		m.Comp,
	)

	m.translationPort.RetrieveIncoming()

	tracing.TraceReqFinalize(trans.translationReq, m.Comp)
	tracing.TraceReqInitiate(translatedReq, m.Comp,
		tracing.MsgIDAtReceiver(reqFromTop, m.Comp))

	return true
}

//nolint:funlen,gocyclo
func (m *middleware) respond() bool {
	rspI := m.bottomPort.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(*sim.GenericMsg)
	var (
		reqFromTop       *sim.GenericMsg
		reqToBottomCombo reqToBottom
		rspToTop         *sim.GenericMsg
	)

	reqInBottom := false

	switch rsp.Payload.(type) {
	case *mem.DataReadyRspPayload:
		reqInBottom = m.isReqInBottomByID(rsp.RspTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RspTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drPayload := sim.MsgPayload[mem.DataReadyRspPayload](rsp)
			rspToTop = mem.DataReadyRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(reqFromTop.Src).
				WithRspTo(reqFromTop.ID).
				WithData(drPayload.Data).
				Build()
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindData,
				"data",
				m.Comp.Name(),
				m.Comp,
			)
		}
	case *mem.WriteDoneRspPayload:
		reqInBottom = m.isReqInBottomByID(rsp.RspTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RspTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(reqFromTop.Src).
				WithRspTo(reqFromTop.ID).
				Build()
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindSubTask,
				"subtask",
				m.Comp.Name(),
				m.Comp,
			)
		}
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp.Payload))
	}

	if reqInBottom {
		err := m.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
			tracing.MilestoneKindNetworkBusy,
			m.topPort.Name(),
			m.Comp.Name(),
			m.Comp,
		)

		m.removeReqToBottomByID(rsp.RspTo)

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, m.Comp)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, m.Comp)
	}

	m.bottomPort.RetrieveIncoming()

	return true
}

func (m *middleware) createTranslatedReq(
	msg *sim.GenericMsg,
	page vm.Page,
) *sim.GenericMsg {
	switch msg.Payload.(type) {
	case *mem.ReadReqPayload:
		return m.createTranslatedReadReq(msg, page)
	case *mem.WriteReqPayload:
		return m.createTranslatedWriteReq(msg, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(msg.Payload))
		return nil
	}
}

func (m *middleware) createTranslatedReadReq(
	msg *sim.GenericMsg,
	page vm.Page,
) *sim.GenericMsg {
	readPayload := sim.MsgPayload[mem.ReadReqPayload](msg)
	offset := readPayload.Address % (1 << m.GetSpec().Log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithAddress(addr).
		WithByteSize(readPayload.AccessByteSize).
		WithPID(0).
		WithInfo(readPayload.Info).
		Build()
	clonePayload := sim.MsgPayload[mem.ReadReqPayload](clone)
	clonePayload.CanWaitForCoalesce = readPayload.CanWaitForCoalesce

	return clone
}

func (m *middleware) createTranslatedWriteReq(
	msg *sim.GenericMsg,
	page vm.Page,
) *sim.GenericMsg {
	writePayload := sim.MsgPayload[mem.WriteReqPayload](msg)
	offset := writePayload.Address % (1 << m.GetSpec().Log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithData(writePayload.Data).
		WithDirtyMask(writePayload.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(writePayload.Info).
		Build()
	clonePayload := sim.MsgPayload[mem.WriteReqPayload](clone)
	clonePayload.CanWaitForCoalesce = writePayload.CanWaitForCoalesce

	return clone
}

func (m *middleware) addrToPageID(addr uint64) uint64 {
	return (addr >> m.GetSpec().Log2PageSize) << m.GetSpec().Log2PageSize
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
		if r.reqToBottom.ID == id {
			return true
		}
	}

	return false
}

func (m *middleware) findReqToBottomByID(id string) reqToBottom {
	for _, r := range m.inflightReqToBottom {
		if r.reqToBottom.ID == id {
			return r
		}
	}

	panic("req to bottom not found")
}

func (m *middleware) removeReqToBottomByID(id string) {
	for i, r := range m.inflightReqToBottom {
		if r.reqToBottom.ID == id {
			m.inflightReqToBottom = append(
				m.inflightReqToBottom[:i],
				m.inflightReqToBottom[i+1:]...)

			return
		}
	}

	panic("req to bottom not found")
}

func (m *middleware) handleCtrlRequest() bool {
	msgI := m.ctrlPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	msg := msgI.(*sim.GenericMsg)
	ctrlPayload := sim.MsgPayload[mem.ControlMsgPayload](msg)

	if ctrlPayload.DiscardTransations {
		return m.handleFlushReq(msg)
	} else if ctrlPayload.Restart {
		return m.handleRestartReq(msg)
	}

	panic("never")
}

func (m *middleware) handleFlushReq(msg *sim.GenericMsg) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(msg.Src).
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

func (m *middleware) handleRestartReq(msg *sim.GenericMsg) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(msg.Src).
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
