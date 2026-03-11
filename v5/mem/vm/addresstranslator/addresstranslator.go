package addresstranslator

import (
	"fmt"
	"io"
	"log"

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

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	ID    string         `json:"id"`
	Src   sim.RemotePort `json:"src"`
	Dst   sim.RemotePort `json:"dst"`
	RspTo string         `json:"rsp_to"`
	Type  string         `json:"type"`
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
	ReqFromTopID   string         `json:"req_from_top_id"`
	ReqFromTopSrc  sim.RemotePort `json:"req_from_top_src"`
	ReqFromTopDst  sim.RemotePort `json:"req_from_top_dst"`
	ReqFromTopType string         `json:"req_from_top_type"`
	ReqToBottomID  string         `json:"req_to_bottom_id"`
	ReqToBottomSrc sim.RemotePort `json:"req_to_bottom_src"`
	ReqToBottomDst sim.RemotePort `json:"req_to_bottom_dst"`
	ReqToBottomType string        `json:"req_to_bottom_type"`
}

// State contains mutable runtime data for the AddressTranslator.
type State struct {
	IsFlushing          bool               `json:"is_flushing"`
	Transactions        []transactionState `json:"transactions"`
	InflightReqToBottom []reqToBottomState `json:"inflight_req_to_bottom"`
}

type transaction struct {
	incomingReqs    []sim.Msg
	translationReq  *vm.TranslationReq
	translationRsp  *vm.TranslationRsp
	translationDone bool
}

type reqToBottom struct {
	reqFromTop  sim.Msg
	reqToBottom sim.Msg
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
			meta := req.Meta()
			ts.IncomingReqs = append(ts.IncomingReqs, incomingReqState{
				ID:    meta.ID,
				Src:   meta.Src,
				Dst:   meta.Dst,
				RspTo: meta.RspTo,
				Type:  fmt.Sprintf("%T", req),
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
		fromMeta := r.reqFromTop.Meta()
		toMeta := r.reqToBottom.Meta()
		state.InflightReqToBottom = append(state.InflightReqToBottom,
			reqToBottomState{
				ReqFromTopID:    fromMeta.ID,
				ReqFromTopSrc:   fromMeta.Src,
				ReqFromTopDst:   fromMeta.Dst,
				ReqFromTopType:  fmt.Sprintf("%T", r.reqFromTop),
				ReqToBottomID:   toMeta.ID,
				ReqToBottomSrc:  toMeta.Src,
				ReqToBottomDst:  toMeta.Dst,
				ReqToBottomType: fmt.Sprintf("%T", r.reqToBottom),
			})
	}

	c.Component.SetState(state)

	return state
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata and
// type string. Only *mem.ReadReq and *mem.WriteReq are used as incoming
// requests in the address translator.
func restoreMemMsg(id string, src, dst sim.RemotePort, rspTo, typ string) sim.Msg {
	switch typ {
	case "*mem.WriteReq":
		m := &mem.WriteReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		return m
	default: // "*mem.ReadReq" or unknown — default to ReadReq
		m := &mem.ReadReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		return m
	}
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
			t.incomingReqs = append(t.incomingReqs,
				restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
					reqState.RspTo, reqState.Type))
		}

		if ts.TranslationReqID != "" {
			t.translationReq = &vm.TranslationReq{
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
			reqFromTop: restoreMemMsg(rs.ReqFromTopID, rs.ReqFromTopSrc,
				rs.ReqFromTopDst, "", rs.ReqFromTopType),
			reqToBottom: restoreMemMsg(rs.ReqToBottomID, rs.ReqToBottomSrc,
				rs.ReqToBottomDst, "", rs.ReqToBottomType),
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

	item := itemI.(mem.AccessReq)
	vAddr := item.GetAddress()
	vPageID := m.addrToPageID(vAddr)

	transReq := vm.TranslationReqBuilder{}.
		WithSrc(m.translationPort.AsRemote()).
		WithDst(m.translationPortMapper.Find(vAddr)).
		WithPID(item.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(m.GetSpec().DeviceID).
		Build()

	err := m.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	trans := &transaction{
		incomingReqs:   []sim.Msg{itemI},
		translationReq: transReq,
	}
	m.transactions = append(m.transactions, trans)

	tracing.TraceReqReceive(itemI, m.Comp)
	tracing.TraceReqInitiate(
		transReq,
		m.Comp,
		tracing.MsgIDAtReceiver(itemI, m.Comp),
	)

	m.topPort.RetrieveIncoming()

	return true
}

func (m *middleware) parseTranslation() bool {
	rspI := m.translationPort.PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(*vm.TranslationRsp)
	trans := m.findTranslationByReqID(rsp.RspTo)

	if trans == nil {
		m.translationPort.RetrieveIncoming()
		return true
	}

	trans.translationRsp = rsp
	trans.translationDone = true

	reqFromTop := trans.incomingReqs[0]
	translatedReq := m.createTranslatedReq(reqFromTop, rsp.Page)

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

	var (
		reqFromTop       sim.Msg
		reqToBottomCombo reqToBottom
		rspToTop         sim.Msg
	)

	reqInBottom := false

	switch rsp := rspI.(type) {
	case *mem.DataReadyRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RspTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RspTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			fromMeta := reqFromTop.Meta()
			rspToTop = mem.DataReadyRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(fromMeta.Src).
				WithRspTo(fromMeta.ID).
				WithData(rsp.Data).
				Build()
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindData,
				"data",
				m.Comp.Name(),
				m.Comp,
			)
		}
	case *mem.WriteDoneRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RspTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RspTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			fromMeta := reqFromTop.Meta()
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(fromMeta.Src).
				WithRspTo(fromMeta.ID).
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
		log.Panicf("cannot handle respond of type %s", fmt.Sprintf("%T", rspI))
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

		rspMeta := rspI.Meta()
		m.removeReqToBottomByID(rspMeta.RspTo)

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, m.Comp)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, m.Comp)
	}

	m.bottomPort.RetrieveIncoming()

	return true
}

func (m *middleware) createTranslatedReq(
	msg sim.Msg,
	page vm.Page,
) sim.Msg {
	switch req := msg.(type) {
	case *mem.ReadReq:
		return m.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return m.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", fmt.Sprintf("%T", msg))
		return nil
	}
}

func (m *middleware) createTranslatedReadReq(
	readReq *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := readReq.Address % (1 << m.GetSpec().Log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithAddress(addr).
		WithByteSize(readReq.AccessByteSize).
		WithPID(0).
		WithInfo(readReq.Info).
		Build()
	clone.CanWaitForCoalesce = readReq.CanWaitForCoalesce

	return clone
}

func (m *middleware) createTranslatedWriteReq(
	writeReq *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := writeReq.Address % (1 << m.GetSpec().Log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithData(writeReq.Data).
		WithDirtyMask(writeReq.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(writeReq.Info).
		Build()
	clone.CanWaitForCoalesce = writeReq.CanWaitForCoalesce

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
		meta := r.reqToBottom.Meta()
		if meta.ID == id {
			return true
		}
	}

	return false
}

func (m *middleware) findReqToBottomByID(id string) reqToBottom {
	for _, r := range m.inflightReqToBottom {
		meta := r.reqToBottom.Meta()
		if meta.ID == id {
			return r
		}
	}

	panic("req to bottom not found")
}

func (m *middleware) removeReqToBottomByID(id string) {
	for i, r := range m.inflightReqToBottom {
		meta := r.reqToBottom.Meta()
		if meta.ID == id {
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

	msg := msgI.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return m.handleFlushReq(msg)
	} else if msg.Restart {
		return m.handleRestartReq(msg)
	}

	panic("never")
}

func (m *middleware) handleFlushReq(msg *mem.ControlMsg) bool {
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

func (m *middleware) handleRestartReq(msg *mem.ControlMsg) bool {
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
