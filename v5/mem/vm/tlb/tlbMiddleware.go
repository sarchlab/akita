package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type pipelineTLBReq struct {
	msg *vm.TranslationReq
}

func (r *pipelineTLBReq) TaskID() string {
	return r.msg.ID
}

type tlbMiddleware struct {
	*Comp
}

func (m *tlbMiddleware) Tick() bool {
	madeProgress := false

	switch m.state {
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

	madeProgress = m.responsePipeline.Tick() || madeProgress

	madeProgress = m.insertIntoPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) insertIntoPipeline() bool {
	madeProgress := false
	spec := m.GetSpec()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !m.responsePipeline.CanAccept() {
			break
		}

		msgI := m.topPort.RetrieveIncoming()
		if msgI == nil {
			break
		}

		msg := msgI.(*vm.TranslationReq)
		m.responsePipeline.Accept(&pipelineTLBReq{
			msg: msg,
		})

		madeProgress = true
	}

	return madeProgress
}

func (m *tlbMiddleware) extractFromPipeline() bool {
	madeProgress := false
	spec := m.GetSpec()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		item := m.responseBuffer.Peek()

		if item == nil {
			break
		}

		msg := item.(*pipelineTLBReq).msg

		ok := m.lookup(msg)
		if ok {
			m.responseBuffer.Pop()

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) handleEnable() bool {
	madeProgress := false
	spec := m.GetSpec()
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
	spec := m.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	if m.mshr.IsEmpty() && m.bottomPort.PeekIncoming() == nil {
		m.state = tlbStatePause
		tracing.AddMilestone(
			m.Comp.Name()+".drain",
			tracing.MilestoneKindHardwareResource,
			m.Comp.Name()+".MSHR",
			m.Comp.Name(),
			m.Comp,
		)
	}

	return madeProgress
}

func (m *tlbMiddleware) respondMSHREntry() bool {
	if m.respondingMSHREntry == nil {
		return false
	}
	mshrEntry := m.respondingMSHREntry
	page := mshrEntry.page
	reqMsg := mshrEntry.Requests[0]
	rspToTop := &vm.TranslationRsp{
		Page: page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort.AsRemote()
	rspToTop.Dst = reqMsg.Src
	rspToTop.RspTo = reqMsg.ID
	rspToTop.TrafficClass = "vm.TranslationRsp"

	err := m.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(reqMsg, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.topPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		m.respondingMSHREntry = nil
	}

	tracing.TraceReqComplete(reqMsg, m.Comp)

	return true
}

func (m *tlbMiddleware) lookup(msg *vm.TranslationReq) bool {
	spec := m.GetSpec()
	mshrEntry := m.mshr.GetEntry(msg.PID, msg.VAddr)
	if mshrEntry != nil {
		return m.processTLBMSHRHit(mshrEntry, msg)
	}
	setID := m.vAddrToSetID(msg.VAddr, spec)
	set := m.sets[setID]
	wayID, page, found := set.Lookup(msg.PID, msg.VAddr)

	if found && page.Valid {
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
	m.visit(setID, wayID)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.Comp),
		tracing.MilestoneKindData,
		m.Comp.Name()+".Sets",
		m.Comp.Name(),
		m.Comp,
	)

	tracing.TraceReqReceive(msg, m.Comp)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(msg, m.Comp), m.Comp, "hit")
	tracing.TraceReqComplete(msg, m.Comp)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(msg *vm.TranslationReq) bool {
	if m.mshr.IsFull() {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.Comp),
		tracing.MilestoneKindHardwareResource,
		m.Comp.Name()+".MSHR",
		m.Comp.Name(),
		m.Comp,
	)

	fetched := m.fetchBottom(msg)
	if fetched {
		tracing.TraceReqReceive(msg, m.Comp)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(msg, m.Comp),
			m.Comp,
			"miss",
		)

		return true
	}
	return false
}

func (m *tlbMiddleware) vAddrToSetID(vAddr uint64, spec Spec) (setID int) {
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
	rsp.Src = m.topPort.AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "vm.TranslationRsp"

	err := m.topPort.Send(rsp)
	if err == nil {
		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(msg, m.Comp),
			tracing.MilestoneKindNetworkBusy,
			m.topPort.Name(),
			m.Comp.Name(),
			m.Comp,
		)
	}
	return err == nil
}

func (m *tlbMiddleware) processTLBMSHRHit(
	mshrEntry *mshrEntry,
	msg *vm.TranslationReq,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, msg)

	tracing.TraceReqReceive(msg, m.Comp)
	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(msg, m.Comp), m.Comp, "mshr-hit")

	return true
}

func (m *tlbMiddleware) fetchBottom(msg *vm.TranslationReq) bool {
	fetchBottom := &vm.TranslationReq{}
	fetchBottom.ID = sim.GetIDGenerator().Generate()
	fetchBottom.Src = m.bottomPort.AsRemote()
	fetchBottom.Dst = m.addressMapper.Find(msg.VAddr)
	fetchBottom.PID = msg.PID
	fetchBottom.VAddr = msg.VAddr
	fetchBottom.DeviceID = msg.DeviceID
	fetchBottom.TrafficClass = "vm.TranslationReq"

	err := m.bottomPort.Send(fetchBottom)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	mshrEntry := m.mshr.Add(msg.PID, msg.VAddr)
	mshrEntry.Requests = append(mshrEntry.Requests, msg)
	mshrEntry.reqToBottom = fetchBottom

	tracing.TraceReqInitiate(fetchBottom, m.Comp,
		tracing.MsgIDAtReceiver(msg, m.Comp))

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
	if m.respondingMSHREntry != nil {
		return false
	}
	itemI := m.bottomPort.PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(*vm.TranslationRsp)
	spec := m.GetSpec()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m.Comp),
		tracing.MilestoneKindData,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)
	page := item.Page

	mshrEntryPresent := m.mshr.IsEntryPresent(page.PID, page.VAddr)
	if !mshrEntryPresent {
		m.bottomPort.RetrieveIncoming()
		return true
	}
	setID := m.vAddrToSetID(page.VAddr, spec)
	set := m.sets[setID]
	wayID, ok := m.sets[setID].Evict()

	if !ok {
		panic("failed to evict")
	}

	set.Update(wayID, page)
	set.Visit(wayID)

	mshrEntry := m.mshr.GetEntry(page.PID, page.VAddr)
	m.respondingMSHREntry = mshrEntry
	mshrEntry.page = page

	m.mshr.Remove(page.PID, page.VAddr)
	m.bottomPort.RetrieveIncoming()
	tracing.TraceReqFinalize(mshrEntry.reqToBottom, m.Comp)

	return true
}

func (m *tlbMiddleware) visit(setID, wayID int) {
	set := m.sets[setID]
	set.Visit(wayID)
}

func (m *tlbMiddleware) handleFlush() bool {
	if m.inflightFlushReq == nil {
		return false
	}

	madeProgress := false
	spec := m.GetSpec()

	if m.mshr.IsEmpty() && m.respondingMSHREntry == nil && m.bottomPort.PeekIncoming() == nil {
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
	spec := m.GetSpec()
	req := m.inflightFlushReq

	rsp := &FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort.AsRemote()
	rsp.Dst = req.Src
	rsp.TrafficClass = "tlb.FlushRsp"

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	for _, vAddr := range req.VAddr {
		setID := m.vAddrToSetID(vAddr, spec)
		set := m.sets[setID]
		wayID, page, found := set.Lookup(req.PID, vAddr)

		if !found {
			continue
		}
		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(req, m.Comp),
			tracing.MilestoneKindDependency,
			m.Comp.Name()+".Sets",
			m.Comp.Name(),
			m.Comp,
		)
		page.Valid = false
		set.Update(wayID, page)
	}

	m.mshr.Reset()

	m.inflightFlushReq = nil
	m.state = tlbStatePause

	return true
}
