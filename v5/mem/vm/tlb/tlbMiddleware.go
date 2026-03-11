package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type tlbMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *tlbMiddleware) Name() string {
	return m.comp.Name()
}

func (m *tlbMiddleware) AcceptHook(hook sim.Hook) {
	m.comp.AcceptHook(hook)
}

func (m *tlbMiddleware) Hooks() []sim.Hook {
	return m.comp.Hooks()
}

func (m *tlbMiddleware) NumHooks() int {
	return m.comp.NumHooks()
}

func (m *tlbMiddleware) InvokeHook(ctx sim.HookCtx) {
	m.comp.InvokeHook(ctx)
}

func (m *tlbMiddleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *tlbMiddleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *tlbMiddleware) controlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *tlbMiddleware) Tick() bool {
	madeProgress := false
	state := m.comp.GetState()

	switch state.TLBState {
	case tlbStateDrain:
		madeProgress = m.handleDrain() || madeProgress
	case tlbStatePause:
		return false
	case tlbStateFlush:
		madeProgress = m.handleFlush() || madeProgress
	default:
		madeProgress = m.handleEnable() || madeProgress
	}
	return madeProgress
}

func (m *tlbMiddleware) processPipeline() bool {
	madeProgress := false

	madeProgress = m.extractFromPipeline() || madeProgress
	madeProgress = m.tickPipeline() || madeProgress
	madeProgress = m.insertIntoPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) tickPipeline() bool {
	next := m.comp.GetNextState()
	stages, progress := pipelineTick(next.PipelineStages, &next.BufferItems)
	next.PipelineStages = stages
	return progress
}

func (m *tlbMiddleware) insertIntoPipeline() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !pipelineCanAccept(next.PipelineStages, spec.PipelineWidth, next.PipelineNumStages) {
			break
		}

		msgI := m.topPort().RetrieveIncoming()
		if msgI == nil {
			break
		}

		msg := msgI.(*vm.TranslationReq)
		next.PipelineStages = pipelineAccept(
			next.PipelineStages, spec.PipelineWidth, next.PipelineNumStages,
			pipelineTLBReqState{Msg: *msg},
		)

		madeProgress = true
	}

	return madeProgress
}

func (m *tlbMiddleware) extractFromPipeline() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if len(next.BufferItems) == 0 {
			break
		}

		item := next.BufferItems[0]
		msg := item.Msg

		ok := m.lookup(&msg)
		if ok {
			next.BufferItems = next.BufferItems[1:]
			madeProgress = true
		} else {
			break
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) handleEnable() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) handleDrain() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	state := m.comp.GetState()
	if mshrIsEmpty(state.MSHREntries) && m.bottomPort().PeekIncoming() == nil {
		next := m.comp.GetNextState()
		next.TLBState = tlbStatePause
		tracing.AddMilestone(
			m.comp.Name()+".drain",
			tracing.MilestoneKindHardwareResource,
			m.comp.Name()+".MSHR",
			m.comp.Name(),
			m,
		)
	}

	return madeProgress
}

func (m *tlbMiddleware) respondMSHREntry() bool {
	state := m.comp.GetState()
	if !state.HasRespondingMSHR {
		return false
	}
	next := m.comp.GetNextState()
	mshrEntry := &next.RespondingMSHRData
	page := mshrEntry.Page
	reqMsg := mshrEntry.Requests[0]
	rspToTop := &vm.TranslationRsp{
		Page: page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = reqMsg.Src
	rspToTop.RspTo = reqMsg.ID
	rspToTop.TrafficClass = "vm.TranslationRsp"

	err := m.topPort().Send(rspToTop)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(&reqMsg, m),
		tracing.MilestoneKindNetworkBusy,
		m.topPort().Name(),
		m.comp.Name(),
		m,
	)

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		next.HasRespondingMSHR = false
	}

	tracing.TraceReqComplete(&reqMsg, m)

	return true
}

func (m *tlbMiddleware) lookup(msg *vm.TranslationReq) bool {
	spec := m.comp.GetSpec()
	state := m.comp.GetState()

	_, found := mshrGetEntry(state.MSHREntries, msg.PID, msg.VAddr)
	if found {
		return m.processTLBMSHRHit(msg)
	}

	setID := vAddrToSetID(msg.VAddr, spec)
	set := state.Sets[setID]
	wayID, page, setFound := setLookup(&set, msg.PID, msg.VAddr)

	if setFound && page.Valid {
		return m.handleTranslationHit(msg, setID, wayID, page)
	}
	return m.handleTranslationMiss(msg)
}

func (m *tlbMiddleware) handleTranslationHit(
	msg *vm.TranslationReq,
	setID, wayID int,
	page vm.Page,
) bool {
	ok := m.sendRspToTop(msg, page)
	if !ok {
		return false
	}

	next := m.comp.GetNextState()
	setVisit(&next.Sets[setID], wayID)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m),
		tracing.MilestoneKindData,
		m.comp.Name()+".Sets",
		m.comp.Name(),
		m,
	)

	tracing.TraceReqReceive(msg, m)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(msg, m), m, "hit")
	tracing.TraceReqComplete(msg, m)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(msg *vm.TranslationReq) bool {
	state := m.comp.GetState()
	spec := m.comp.GetSpec()

	if mshrIsFull(state.MSHREntries, spec.MSHRSize) {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m),
		tracing.MilestoneKindHardwareResource,
		m.comp.Name()+".MSHR",
		m.comp.Name(),
		m,
	)

	fetched := m.fetchBottom(msg)
	if fetched {
		tracing.TraceReqReceive(msg, m)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(msg, m),
			m,
			"miss",
		)

		return true
	}
	return false
}

func vAddrToSetID(vAddr uint64, spec Spec) (setID int) {
	return int(vAddr / spec.PageSize % uint64(spec.NumSets))
}

func (m *tlbMiddleware) sendRspToTop(
	msg *vm.TranslationReq,
	page vm.Page,
) bool {
	rsp := &vm.TranslationRsp{
		Page: page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "vm.TranslationRsp"

	err := m.topPort().Send(rsp)
	if err == nil {
		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(msg, m),
			tracing.MilestoneKindNetworkBusy,
			m.topPort().Name(),
			m.comp.Name(),
			m,
		)
	}
	return err == nil
}

func (m *tlbMiddleware) processTLBMSHRHit(
	msg *vm.TranslationReq,
) bool {
	next := m.comp.GetNextState()
	idx, found := mshrGetEntry(next.MSHREntries, msg.PID, msg.VAddr)
	if !found {
		return false
	}
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, *msg)

	tracing.TraceReqReceive(msg, m)
	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(msg, m), m, "mshr-hit")

	return true
}

func (m *tlbMiddleware) fetchBottom(msg *vm.TranslationReq) bool {
	spec := m.comp.GetSpec()

	fetchBottom := &vm.TranslationReq{}
	fetchBottom.ID = sim.GetIDGenerator().Generate()
	fetchBottom.Src = m.bottomPort().AsRemote()
	fetchBottom.Dst = findTranslationPort(spec, msg.VAddr)
	fetchBottom.PID = msg.PID
	fetchBottom.VAddr = msg.VAddr
	fetchBottom.DeviceID = msg.DeviceID
	fetchBottom.TrafficClass = "vm.TranslationReq"

	err := m.bottomPort().Send(fetchBottom)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort().Name(),
		m.comp.Name(),
		m,
	)

	next := m.comp.GetNextState()
	var idx int
	next.MSHREntries, idx = mshrAdd(next.MSHREntries, spec.MSHRSize, msg.PID, msg.VAddr)
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, *msg)
	next.MSHREntries[idx].HasReqToBottom = true
	next.MSHREntries[idx].ReqToBottom = *fetchBottom

	tracing.TraceReqInitiate(fetchBottom, m,
		tracing.MsgIDAtReceiver(msg, m))

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
	state := m.comp.GetState()
	if state.HasRespondingMSHR {
		return false
	}
	itemI := m.bottomPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(*vm.TranslationRsp)
	spec := m.comp.GetSpec()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m),
		tracing.MilestoneKindData,
		m.bottomPort().Name(),
		m.comp.Name(),
		m,
	)
	page := item.Page

	if !mshrIsEntryPresent(state.MSHREntries, page.PID, page.VAddr) {
		m.bottomPort().RetrieveIncoming()
		return true
	}

	next := m.comp.GetNextState()
	setID := vAddrToSetID(page.VAddr, spec)
	wayID, ok := setEvict(&next.Sets[setID])

	if !ok {
		panic("failed to evict")
	}

	setUpdate(&next.Sets[setID], wayID, page)
	setVisit(&next.Sets[setID], wayID)

	mshrIdx, _ := mshrGetEntry(next.MSHREntries, page.PID, page.VAddr)
	next.HasRespondingMSHR = true
	next.RespondingMSHRData = next.MSHREntries[mshrIdx]
	next.RespondingMSHRData.Page = page

	// Get reqToBottom for tracing before removing
	reqToBottom := next.MSHREntries[mshrIdx].ReqToBottom

	next.MSHREntries = mshrRemove(next.MSHREntries, page.PID, page.VAddr)
	m.bottomPort().RetrieveIncoming()

	if next.RespondingMSHRData.HasReqToBottom {
		tracing.TraceReqFinalize(&reqToBottom, m)
	}

	return true
}

func (m *tlbMiddleware) handleFlush() bool {
	state := m.comp.GetState()
	if !state.HasInflightFlushReq {
		return false
	}

	madeProgress := false
	spec := m.comp.GetSpec()

	if mshrIsEmpty(state.MSHREntries) && !state.HasRespondingMSHR && m.bottomPort().PeekIncoming() == nil {
		madeProgress = m.processTLBFlush() || madeProgress
		return madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) processTLBFlush() bool {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	req := next.InflightFlushReqMsg

	rsp := &FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = req.Src
	rsp.TrafficClass = "tlb.FlushRsp"

	err := m.controlPort().Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(&req, m),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m,
	)

	for _, vAddr := range req.VAddr {
		setID := vAddrToSetID(vAddr, spec)
		wayID, page, found := setLookup(&next.Sets[setID], req.PID, vAddr)

		if !found {
			continue
		}
		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(&req, m),
			tracing.MilestoneKindDependency,
			m.comp.Name()+".Sets",
			m.comp.Name(),
			m,
		)
		page.Valid = false
		setUpdate(&next.Sets[setID], wayID, page)
	}

	next.MSHREntries = nil

	next.HasInflightFlushReq = false
	next.TLBState = tlbStatePause

	return true
}
