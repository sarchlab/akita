package addresstranslator

import (
	"fmt"
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

	MemMapperKind             string           `json:"mem_mapper_kind"`
	MemMapperPorts            []sim.RemotePort `json:"mem_mapper_ports"`
	MemMapperInterleavingSize uint64           `json:"mem_mapper_interleaving_size"`

	TransMapperKind             string           `json:"trans_mapper_kind"`
	TransMapperPorts            []sim.RemotePort `json:"trans_mapper_ports"`
	TransMapperInterleavingSize uint64           `json:"trans_mapper_interleaving_size"`
}

// incomingReqState is a serializable representation of an incoming request.
type incomingReqState struct {
	ID    string         `json:"id"`
	Src   sim.RemotePort `json:"src"`
	Dst   sim.RemotePort `json:"dst"`
	RspTo string         `json:"rsp_to"`
	Type  string         `json:"type"`

	// Fields preserved for translated request creation.
	Address            uint64 `json:"address"`
	AccessByteSize     uint64 `json:"access_byte_size"`
	PID                vm.PID `json:"pid"`
	Data               []byte `json:"data,omitempty"`
	DirtyMask          []bool `json:"dirty_mask,omitempty"`
	CanWaitForCoalesce bool   `json:"can_wait_for_coalesce"`
}

// transactionState is a serializable representation of a runtime transaction.
type transactionState struct {
	IncomingReqs      []incomingReqState `json:"incoming_reqs"`
	TranslationReqID  string             `json:"translation_req_id"`
	TranslationReqSrc sim.RemotePort     `json:"translation_req_src"`
	TranslationReqDst sim.RemotePort     `json:"translation_req_dst"`
	TranslationDone   bool               `json:"translation_done"`
	Page              vm.Page            `json:"page"`
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

// findMemoryPort implements the same Find() logic as SinglePortMapper and
// InterleavedAddressPortMapper, using Spec fields.
func findMemoryPort(spec Spec, addr uint64) sim.RemotePort {
	switch spec.MemMapperKind {
	case "single":
		return spec.MemMapperPorts[0]
	case "interleaved":
		n := addr / spec.MemMapperInterleavingSize %
			uint64(len(spec.MemMapperPorts))
		return spec.MemMapperPorts[n]
	default:
		panic("invalid mem mapper kind: " + spec.MemMapperKind)
	}
}

// findTranslationPort implements the same Find() logic as SinglePortMapper and
// InterleavedAddressPortMapper, using Spec fields.
func findTranslationPort(spec Spec, addr uint64) sim.RemotePort {
	switch spec.TransMapperKind {
	case "single":
		return spec.TransMapperPorts[0]
	case "interleaved":
		n := addr / spec.TransMapperInterleavingSize %
			uint64(len(spec.TransMapperPorts))
		return spec.TransMapperPorts[n]
	default:
		panic("invalid trans mapper kind: " + spec.TransMapperKind)
	}
}

type middleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *middleware) Name() string {
	return m.comp.Name()
}

func (m *middleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *middleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *middleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *middleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *middleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *middleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *middleware) translationPort() sim.Port {
	return m.comp.GetPortByName("Translation")
}

func (m *middleware) ctrlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

// Tick updates state at each cycle.
func (m *middleware) Tick() bool {
	madeProgress := false

	state := m.comp.GetState()
	if !state.IsFlushing {
		madeProgress = m.runPipeline()
	} else {
		spec := m.comp.GetSpec()
		for i := 0; i < spec.NumReqPerCycle; i++ {
			madeProgress = m.parseTranslation() || madeProgress
		}
	}

	madeProgress = m.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	spec := m.comp.GetSpec()

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
	itemI := m.topPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(mem.AccessReq)
	vAddr := item.GetAddress()
	spec := m.comp.GetSpec()
	vPageID := addrToPageID(vAddr, spec.Log2PageSize)

	transReq := &vm.TranslationReq{}
	transReq.ID = sim.GetIDGenerator().Generate()
	transReq.Src = m.translationPort().AsRemote()
	transReq.Dst = findTranslationPort(spec, vAddr)
	transReq.PID = item.GetPID()
	transReq.VAddr = vPageID
	transReq.DeviceID = spec.DeviceID
	transReq.TrafficClass = "vm.TranslationReq"

	err := m.translationPort().Send(transReq)
	if err != nil {
		return false
	}

	incoming := msgToIncomingReqState(itemI)

	nextState := m.comp.GetNextState()
	trans := transactionState{
		IncomingReqs:      []incomingReqState{incoming},
		TranslationReqID:  transReq.ID,
		TranslationReqSrc: transReq.Src,
		TranslationReqDst: transReq.Dst,
	}
	nextState.Transactions = append(nextState.Transactions, trans)

	tracing.TraceReqReceive(itemI, m)
	tracing.TraceReqInitiate(
		transReq,
		m,
		tracing.MsgIDAtReceiver(itemI, m),
	)

	m.topPort().RetrieveIncoming()

	return true
}

func (m *middleware) parseTranslation() bool {
	rspI := m.translationPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(*vm.TranslationRsp)
	cur := m.comp.GetState()
	transIdx := findTransactionByReqID(cur.Transactions, rsp.RspTo)

	if transIdx < 0 {
		m.translationPort().RetrieveIncoming()
		return true
	}

	curTrans := &cur.Transactions[transIdx]
	reqState := curTrans.IncomingReqs[0]
	spec := m.comp.GetSpec()
	translatedReq := createTranslatedReq(reqState, rsp.Page,
		spec.Log2PageSize, m.bottomPort().AsRemote(), spec)

	err := m.bottomPort().Send(translatedReq)
	if err != nil {
		return false
	}

	nextState := m.comp.GetNextState()
	nextTrans := &nextState.Transactions[transIdx]
	nextTrans.TranslationDone = true
	nextTrans.Page = rsp.Page

	reqToBot := buildReqToBottom(reqState, translatedReq)
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)
	nextTrans.IncomingReqs = nextTrans.IncomingReqs[1:]

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.traceTranslationComplete(nextTrans, reqState, translatedReq)

	m.translationPort().RetrieveIncoming()

	return true
}

// buildReqToBottom creates a reqToBottomState from the incoming request
// and the translated outgoing request.
func buildReqToBottom(
	reqState incomingReqState, translatedReq sim.Msg,
) reqToBottomState {
	return reqToBottomState{
		ReqFromTopID:    reqState.ID,
		ReqFromTopSrc:   reqState.Src,
		ReqFromTopDst:   reqState.Dst,
		ReqFromTopType:  reqState.Type,
		ReqToBottomID:   translatedReq.Meta().ID,
		ReqToBottomSrc:  translatedReq.Meta().Src,
		ReqToBottomDst:  translatedReq.Meta().Dst,
		ReqToBottomType: fmt.Sprintf("%T", translatedReq),
	}
}

// traceTranslationComplete records tracing milestones for a completed
// translation and initiates the downstream request trace.
func (m *middleware) traceTranslationComplete(
	trans *transactionState,
	reqState incomingReqState,
	translatedReq sim.Msg,
) {
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(translatedReq, m),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort().Name(),
		m.comp.Name(),
		m,
	)

	fakeFromTop := restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
		reqState.RspTo, reqState.Type)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(fakeFromTop, m),
		tracing.MilestoneKindTranslation,
		"translation",
		m.comp.Name(),
		m,
	)

	fakeTransReq := &vm.TranslationReq{}
	fakeTransReq.ID = trans.TranslationReqID
	fakeTransReq.Src = trans.TranslationReqSrc
	fakeTransReq.Dst = trans.TranslationReqDst
	tracing.TraceReqFinalize(fakeTransReq, m)
	tracing.TraceReqInitiate(translatedReq, m,
		tracing.MsgIDAtReceiver(fakeFromTop, m))
}

//nolint:funlen,gocyclo
func (m *middleware) respond() bool {
	rspI := m.bottomPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	cur := m.comp.GetState()

	var (
		reqFromTopState reqToBottomState
		rspToTop        sim.Msg
	)

	reqInBottom := false

	switch rsp := rspI.(type) {
	case *mem.DataReadyRsp:
		reqInBottom = isReqInBottomByID(cur.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(cur.InflightReqToBottom, rsp.RspTo)
			rspToTop = &mem.DataReadyRsp{
				Data: rsp.Data,
			}
			rspToTop.Meta().ID = sim.GetIDGenerator().Generate()
			rspToTop.Meta().Src = m.topPort().AsRemote()
			rspToTop.Meta().Dst = reqFromTopState.ReqFromTopSrc
			rspToTop.Meta().RspTo = reqFromTopState.ReqFromTopID
			rspToTop.Meta().TrafficBytes = len(rsp.Data) + 4
			rspToTop.Meta().TrafficClass = "mem.DataReadyRsp"

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				"", reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(fakeFromTop, m),
				tracing.MilestoneKindData,
				"data",
				m.comp.Name(),
				m,
			)
		}
	case *mem.WriteDoneRsp:
		reqInBottom = isReqInBottomByID(cur.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(cur.InflightReqToBottom, rsp.RspTo)
			rspToTop = &mem.WriteDoneRsp{}
			rspToTop.Meta().ID = sim.GetIDGenerator().Generate()
			rspToTop.Meta().Src = m.topPort().AsRemote()
			rspToTop.Meta().Dst = reqFromTopState.ReqFromTopSrc
			rspToTop.Meta().RspTo = reqFromTopState.ReqFromTopID
			rspToTop.Meta().TrafficBytes = 4
			rspToTop.Meta().TrafficClass = "mem.WriteDoneRsp"

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				"", reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(fakeFromTop, m),
				tracing.MilestoneKindSubTask,
				"subtask",
				m.comp.Name(),
				m,
			)
		}
	default:
		log.Panicf("cannot handle respond of type %s", fmt.Sprintf("%T", rspI))
	}

	if reqInBottom {
		err := m.topPort().Send(rspToTop)
		if err != nil {
			return false
		}

		fakeFromTop := restoreMemMsg(
			reqFromTopState.ReqFromTopID,
			reqFromTopState.ReqFromTopSrc,
			reqFromTopState.ReqFromTopDst,
			"", reqFromTopState.ReqFromTopType)

		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(fakeFromTop, m),
			tracing.MilestoneKindNetworkBusy,
			m.topPort().Name(),
			m.comp.Name(),
			m,
		)

		nextState := m.comp.GetNextState()
		rspMeta := rspI.Meta()
		removeReqToBottomByID(nextState, rspMeta.RspTo)

		fakeReqToBottom := restoreMemMsg(
			reqFromTopState.ReqToBottomID,
			reqFromTopState.ReqToBottomSrc,
			reqFromTopState.ReqToBottomDst,
			"", reqFromTopState.ReqToBottomType)
		tracing.TraceReqFinalize(fakeReqToBottom, m)
		tracing.TraceReqComplete(fakeFromTop, m)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}

func (m *middleware) handleCtrlRequest() bool {
	msgI := m.ctrlPort().PeekIncoming()
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
	rsp := &mem.ControlMsg{
		NotifyDone: true,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.ControlMsg"

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	m.ctrlPort().RetrieveIncoming()

	nextState := m.comp.GetNextState()
	nextState.Transactions = nil
	nextState.InflightReqToBottom = nil
	nextState.IsFlushing = true

	return true
}

func (m *middleware) handleRestartReq(msg *mem.ControlMsg) bool {
	rsp := &mem.ControlMsg{
		NotifyDone: true,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.ControlMsg"

	err := m.ctrlPort().Send(rsp)

	if err != nil {
		return false
	}

	for m.topPort().RetrieveIncoming() != nil {
	}

	for m.bottomPort().RetrieveIncoming() != nil {
	}

	for m.translationPort().RetrieveIncoming() != nil {
	}

	nextState := m.comp.GetNextState()
	nextState.IsFlushing = false

	m.ctrlPort().RetrieveIncoming()

	return true
}

// Helper functions

func addrToPageID(addr, log2PageSize uint64) uint64 {
	return (addr >> log2PageSize) << log2PageSize
}

func msgToIncomingReqState(msg sim.Msg) incomingReqState {
	meta := msg.Meta()
	s := incomingReqState{
		ID:    meta.ID,
		Src:   meta.Src,
		Dst:   meta.Dst,
		RspTo: meta.RspTo,
		Type:  fmt.Sprintf("%T", msg),
	}

	switch req := msg.(type) {
	case *mem.ReadReq:
		s.Address = req.Address
		s.AccessByteSize = req.AccessByteSize
		s.PID = req.PID
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	case *mem.WriteReq:
		s.Address = req.Address
		s.PID = req.PID
		s.Data = req.Data
		s.DirtyMask = req.DirtyMask
		s.CanWaitForCoalesce = req.CanWaitForCoalesce
	default:
		log.Panicf("cannot convert message of type %T", msg)
	}

	return s
}

func createTranslatedReq(
	reqState incomingReqState,
	page vm.Page,
	log2PageSize uint64,
	bottomPortRemote sim.RemotePort,
	spec Spec,
) sim.Msg {
	offset := reqState.Address % (1 << log2PageSize)
	addr := page.PAddr + offset

	switch reqState.Type {
	case "*mem.ReadReq":
		clone := &mem.ReadReq{}
		clone.ID = sim.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = findMemoryPort(spec, addr)
		clone.Address = addr
		clone.AccessByteSize = reqState.AccessByteSize
		clone.PID = 0
		clone.TrafficBytes = 12
		clone.TrafficClass = "mem.ReadReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	case "*mem.WriteReq":
		clone := &mem.WriteReq{}
		clone.ID = sim.GetIDGenerator().Generate()
		clone.Src = bottomPortRemote
		clone.Dst = findMemoryPort(spec, addr)
		clone.Data = reqState.Data
		clone.DirtyMask = reqState.DirtyMask
		clone.Address = addr
		clone.PID = 0
		clone.TrafficBytes = len(reqState.Data) + 12
		clone.TrafficClass = "mem.WriteReq"
		clone.CanWaitForCoalesce = reqState.CanWaitForCoalesce
		return clone
	default:
		log.Panicf("cannot translate request of type %s", reqState.Type)
		return nil
	}
}

// restoreMemMsg reconstructs a concrete mem message from saved metadata.
func restoreMemMsg(id string, src, dst sim.RemotePort, rspTo, typ string) sim.Msg {
	switch typ {
	case "*mem.WriteReq":
		m := &mem.WriteReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		return m
	default:
		m := &mem.ReadReq{}
		m.ID = id
		m.Src = src
		m.Dst = dst
		m.RspTo = rspTo
		return m
	}
}

func findTransactionByReqID(transactions []transactionState, id string) int {
	for i, t := range transactions {
		if t.TranslationReqID == id {
			return i
		}
	}
	return -1
}

func removeTransaction(state *State, idx int) {
	state.Transactions = append(
		state.Transactions[:idx],
		state.Transactions[idx+1:]...)
}

func isReqInBottomByID(inflight []reqToBottomState, id string) bool {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return true
		}
	}
	return false
}

func findReqToBottomByID(inflight []reqToBottomState, id string) reqToBottomState {
	for _, r := range inflight {
		if r.ReqToBottomID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func removeReqToBottomByID(state *State, id string) {
	for i, r := range state.InflightReqToBottom {
		if r.ReqToBottomID == id {
			state.InflightReqToBottom = append(
				state.InflightReqToBottom[:i],
				state.InflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}
