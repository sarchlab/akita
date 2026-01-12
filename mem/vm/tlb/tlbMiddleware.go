package tlb

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/tracing"
)

type pipelineTLBReq struct {
	req *vm.TranslationReq
}

func (r *pipelineTLBReq) TaskID() string {
	return r.req.ID
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

// get req from port buffer and insert into pipeline
func (m *tlbMiddleware) insertIntoPipeline() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		if !m.responsePipeline.CanAccept() {
			break
		}

		req := m.topPort.RetrieveIncoming()
		if req == nil {
			break
		}

		m.responsePipeline.Accept(&pipelineTLBReq{
			req: req.(*vm.TranslationReq),
		})

		madeProgress = true
	}

	return madeProgress
}

func (m *tlbMiddleware) extractFromPipeline() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		item := m.responseBuffer.Peek()

		if item == nil {
			break
		}

		req := item.(*pipelineTLBReq).req

		ok := m.lookup(req)
		if ok {
			m.responseBuffer.Pop()

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) handleEnable() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) handleDrain() bool {
	madeProgress := false
	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
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
	req := mshrEntry.Requests[0]
	rspToTop := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()

	err := m.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.topPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		m.respondingMSHREntry = nil
	}

	tracing.TraceReqComplete(req, m.Comp)

	return true
}

func (m *tlbMiddleware) lookup(req *vm.TranslationReq) bool {
	mshrEntry := m.mshr.GetEntry(req.PID, req.VAddr)
	if mshrEntry != nil {
		return m.processTLBMSHRHit(mshrEntry, req)
	}
	setID := m.vAddrToSetID(req.VAddr)
	set := m.sets[setID]
	wayID, page, found := set.Lookup(req.PID, req.VAddr)

	if found && page.Valid {
		return m.handleTranslationHit(req, setID, wayID, page)
	}
	return m.handleTranslationMiss(req)
}

func (m *tlbMiddleware) handleTranslationHit(
	req *vm.TranslationReq,
	setID, wayID int,
	page vm.Page,
) bool {
	ok := m.sendRspToTop(req, page)
	if !ok {
		return false
	}
	m.visit(setID, wayID)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindData,
		m.Comp.Name()+".Sets",
		m.Comp.Name(),
		m.Comp,
	)

	tracing.TraceReqReceive(req, m.Comp)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, m.Comp), m.Comp, "hit")
	tracing.TraceReqComplete(req, m.Comp)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(
	req *vm.TranslationReq,
) bool {
	if m.mshr.IsFull() {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindHardwareResource,
		m.Comp.Name()+".MSHR",
		m.Comp.Name(),
		m.Comp,
	)

	fetched := m.fetchBottom(req)
	if fetched {
		tracing.TraceReqReceive(req, m.Comp)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(req, m.Comp),
			m.Comp,
			"miss",
		)

		return true
	}
	return false
}

func (m *tlbMiddleware) vAddrToSetID(vAddr uint64) (setID int) {
	return int(vAddr / m.pageSize % uint64(m.numSets))
}

func (m *tlbMiddleware) sendRspToTop(
	req *vm.TranslationReq,
	page vm.Page,
) bool {
	rsp := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()

	err := m.topPort.Send(rsp)
	if err == nil {
		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(req, m.Comp),
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
	req *vm.TranslationReq,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, req)

	tracing.TraceReqReceive(req, m.Comp)
	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(req, m.Comp), m.Comp, "mshr-hit")

	return true
}

func (m *tlbMiddleware) fetchBottom(req *vm.TranslationReq) bool {
	fetchBottom := vm.TranslationReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.addressMapper.Find(req.VAddr)).
		WithPID(req.PID).
		WithVAddr(req.VAddr).
		WithDeviceID(req.DeviceID).
		Build()

	err := m.bottomPort.Send(fetchBottom)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	mshrEntry := m.mshr.Add(req.PID, req.VAddr)
	mshrEntry.Requests = append(mshrEntry.Requests, req)
	mshrEntry.reqToBottom = fetchBottom

	tracing.TraceReqInitiate(fetchBottom, m.Comp,
		tracing.MsgIDAtReceiver(req, m.Comp))

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
	if m.respondingMSHREntry != nil {
		return false
	}
	item := m.bottomPort.PeekIncoming()
	if item == nil {
		return false
	}

	rsp := item.(*vm.TranslationRsp)
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(rsp, m.Comp),
		tracing.MilestoneKindData,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)
	page := rsp.Page

	mshrEntryPresent := m.mshr.IsEntryPresent(rsp.Page.PID, rsp.Page.VAddr)
	if !mshrEntryPresent {
		m.bottomPort.RetrieveIncoming()
		return true
	}
	setID := m.vAddrToSetID(page.VAddr)
	set := m.sets[setID]
	wayID, ok := m.sets[setID].Evict()

	if !ok {
		panic("failed to evict")
	}

	set.Update(wayID, page)
	set.Visit(wayID)

	mshrEntry := m.mshr.GetEntry(rsp.Page.PID, rsp.Page.VAddr)
	m.respondingMSHREntry = mshrEntry
	mshrEntry.page = page

	m.mshr.Remove(rsp.Page.PID, rsp.Page.VAddr)
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

	if m.mshr.IsEmpty() && m.respondingMSHREntry == nil && m.bottomPort.PeekIncoming() == nil {
		madeProgress = m.processTLBFlush() || madeProgress
		return madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) processTLBFlush() bool {
	req := m.inflightFlushReq

	rsp := FlushRspBuilder{}.
		WithSrc(m.controlPort.AsRemote()).
		WithDst(req.Src).
		Build()

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
		setID := m.vAddrToSetID(vAddr)
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
