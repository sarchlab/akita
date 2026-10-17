package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type tlbMiddleware struct {
	comp *Comp
}

func (m *tlbMiddleware) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *tlbMiddleware) bottomPort() messaging.Port {
	return m.comp.Ports.Bottom
}

func (m *tlbMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false
	next := &m.comp.State

	switch next.TLBState {
	case tlbStateDrain:
		madeProgress = m.handleDrain() || madeProgress
	case tlbStatePause:
		return false
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
	next := &m.comp.State
	return next.Pipeline.Tick(&next.BufferItems)
}

func (m *tlbMiddleware) insertIntoPipeline() bool {
	madeProgress := false
	spec := m.comp.Spec
	next := &m.comp.State

	for i := 0; i < spec.NumReqPerCycle; i++ {
		// Check for a head message before testing pipeline capacity. The port
		// attributes the admission milestone to its own head task.
		_, ok := m.topPort().PeekIncoming()
		if !ok {
			break
		}

		if !next.Pipeline.CanAccept() {
			break
		}

		// A pipeline slot is free: the hardware-resource wait the request spent at
		// the head of the Top buffer is over. This admission wait belongs to the
		// incoming-buffer task; req_in (opened at retrieve, below) covers only the
		// processing that follows.
		m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".pipeline"))

		msgI, _ := m.topPort().RetrieveIncoming()
		msg := msgI

		// Admit the request: open req_in at retrieve, then open the pipeline
		// subtask as a child of req_in so the pipeline latency is attributed
		// rather than left as a gap between the buffer task and the post-lookup
		// milestones.
		recvTaskID := tasks.StartReqIn(m.comp, msg)

		pid := tasks.StartTask(m.comp, tasks.StartInfo{
			ParentID: recvTaskID,
			Kind:     tasks.KindPipeline,
			What:     m.comp.Name() + ".pipeline",
		})

		// Each stage takes one cycle, so a request spends spec.Latency
		// cycles in the pipeline and a new request can enter every cycle.
		next.Pipeline.Accept(pipelineTLBReqState{
			Msg: msg, RecvTaskID: recvTaskID, PipelineTaskID: pid,
		})

		madeProgress = true
	}

	return madeProgress
}

func (m *tlbMiddleware) extractFromPipeline() bool {
	madeProgress := false
	spec := m.comp.Spec
	next := &m.comp.State

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if next.BufferItems.Size() == 0 {
			break
		}

		item, _ := next.BufferItems.Peek()

		// The pipeline traversal is done; the request is now being looked up.
		// Mark that interval as work (not a blocking reason) before lookup, so
		// it is not absorbed by a same-tick MSHR/hit milestone emitted inside
		// lookup. Re-emitted on lookup retries; the (Kind, What) dedup keeps the
		// first.
		tasks.AddMilestone(m.comp, tasks.Milestone{
			TaskID: item.RecvTaskID,
			Kind:   tasks.MilestoneKindWork,
			What:   m.comp.Name() + ".pipeline",
		})

		ok := m.lookup(item)
		if ok {
			next.BufferItems.Pop()
			// The request has left the pipeline (it is now being looked up):
			// close the pipeline subtask opened at admission.
			tasks.EndTask(m.comp, tasks.EndInfo{ID: item.PipelineTaskID})
			madeProgress = true
		} else {
			break
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) handleEnable() bool {
	madeProgress := false
	spec := m.comp.Spec
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
	spec := m.comp.Spec
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	// Draining advances in-flight pipeline work but admits no new Top
	// requests (insertIntoPipeline), per the protocol's "Drain stops
	// accepting new traffic"; queued requests resume after Enable.
	madeProgress = m.extractFromPipeline() || madeProgress
	madeProgress = m.tickPipeline() || madeProgress

	next := &m.comp.State
	// Stay draining until the last fetched page has also been responded to
	// the top. parseBottom stages that response in RespondingMSHRData (and
	// empties MSHREntries) before respondMSHREntry can drain it, so pausing
	// on mshrIsEmpty alone would strand the final translation response.
	if mshrIsEmpty(next.MSHREntries) && !next.HasRespondingMSHR {
		_, hasBottom := m.bottomPort().PeekIncoming()
		if !hasBottom {
			next.TLBState = tlbStatePause
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) respondMSHREntry() bool {
	next := &m.comp.State
	if !next.HasRespondingMSHR {
		return false
	}
	if len(next.RespondingMSHRData.Requests) == 0 {
		next.HasRespondingMSHR = false
		return false
	}
	mshrEntry := &next.RespondingMSHRData
	page := mshrEntry.Page
	req := mshrEntry.Requests[0]
	reqMsg := req.Msg
	rspToTop := messaging.Msg{Payload: vmprotocol.TranslationRsp{
		Page: page,
	},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          reqMsg.Src,
		RspTo:        reqMsg.ID,
		TrafficClass: "vmprotocol.TranslationRsp"}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rspToTop)

	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: req.RecvTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		next.HasRespondingMSHR = false
	}

	tasks.EndReqIn(m.comp, req.RecvTaskID)

	return true
}

func (m *tlbMiddleware) lookup(item pipelineTLBReqState) bool {
	msg := item.Msg
	request := msg.Payload.(vmprotocol.TranslationReq)

	spec := m.comp.Spec
	next := &m.comp.State

	_, found := mshrGetEntry(next.MSHREntries, request.PID, request.VAddr)
	if found {
		return m.processTLBMSHRHit(item)
	}

	setID := vAddrToSetID(request.VAddr, spec)
	wayID, page, setFound := setLookup(&next.Sets[setID], request.PID, request.VAddr)

	if setFound && page.Valid {
		return m.handleTranslationHit(item, setID, wayID, page)
	}
	return m.handleTranslationMiss(item)
}

func (m *tlbMiddleware) handleTranslationHit(
	item pipelineTLBReqState,
	setID, wayID int,
	page vm.Page,
) bool {
	ok := m.sendRspToTop(item, page)
	if !ok {
		return false
	}

	next := &m.comp.State
	setVisit(&next.Sets[setID], wayID)

	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: item.RecvTaskID,
		Kind:   tasks.MilestoneKindData,
		What:   m.comp.Name() + ".Sets",
	})

	tasks.AddTag(m.comp, tasks.Tag{
		TaskID: item.RecvTaskID,
		What:   "hit",
	})
	tasks.EndReqIn(m.comp, item.RecvTaskID)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(item pipelineTLBReqState) bool {
	next := &m.comp.State
	spec := m.comp.Spec

	if mshrIsFull(next.MSHREntries, spec.MSHRSize) {
		return false
	}

	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: item.RecvTaskID,
		Kind:   tasks.MilestoneKindHardwareResource,
		What:   m.comp.Name() + ".MSHR",
	})

	fetched := m.fetchBottom(item)
	if fetched {
		tasks.AddTag(m.comp, tasks.Tag{
			TaskID: item.RecvTaskID,
			What:   "miss",
		})

		return true
	}
	return false
}

func vAddrToSetID(vAddr uint64, spec Spec) (setID int) {
	return int(vAddr / spec.pageSize() % uint64(spec.NumSets))
}

func (m *tlbMiddleware) sendRspToTop(
	item pipelineTLBReqState,
	page vm.Page,
) bool {
	msg := item.Msg
	rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
		Page: page,
	},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          msg.Src,
		RspTo:        msg.ID,
		TrafficClass: "vmprotocol.TranslationRsp"}

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)
	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: item.RecvTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})
	return true
}

func (m *tlbMiddleware) processTLBMSHRHit(
	item pipelineTLBReqState,
) bool {
	msg := item.Msg
	request := msg.Payload.(vmprotocol.TranslationReq)

	next := &m.comp.State
	idx, found := mshrGetEntry(next.MSHREntries, request.PID, request.VAddr)
	if !found {
		return false
	}
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, item)

	tasks.AddTag(m.comp, tasks.Tag{
		TaskID: item.RecvTaskID,
		What:   "mshr-hit",
	})

	return true
}

func (m *tlbMiddleware) fetchBottom(item pipelineTLBReqState) bool {
	msg := item.Msg
	request := msg.Payload.(vmprotocol.TranslationReq)

	spec := m.comp.Spec

	fetchBottom := messaging.Msg{Payload: vmprotocol.TranslationReq{
		PID:      request.PID,
		VAddr:    request.VAddr,
		DeviceID: request.DeviceID},
		ID:  m.comp.NewID(),
		Src: m.bottomPort().AsRemote(),
		Dst: spec.Bottom.Find(request.VAddr),

		TrafficClass: "vmprotocol.TranslationReq"}

	if !m.bottomPort().CanSend() {
		return false
	}

	sendTaskID := tasks.StartReqOut(m.comp, fetchBottom, item.RecvTaskID)
	m.bottomPort().Send(fetchBottom)

	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: item.RecvTaskID,
		Kind:   tasks.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	next := &m.comp.State
	var idx int
	next.MSHREntries, idx = mshrAdd(next.MSHREntries, spec.MSHRSize, request.PID, request.VAddr)
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, item)
	next.MSHREntries[idx].HasReqToBottom = true
	next.MSHREntries[idx].ReqToBottom = fetchBottom
	next.MSHREntries[idx].SendTaskID = sendTaskID

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
	next := &m.comp.State
	if next.HasRespondingMSHR {
		return false
	}
	itemI, ok := m.bottomPort().PeekIncoming()
	if !ok {
		return false
	}

	item := itemI
	spec := m.comp.Spec
	page := item.Payload.(vmprotocol.TranslationRsp).Page

	mshrIdx, found := mshrGetEntry(next.MSHREntries, page.PID, page.VAddr)
	if !found || next.MSHREntries[mshrIdx].ReqToBottom.ID != item.RspTo {
		// Either no request is outstanding for this page, or this is a stale
		// response whose request was discarded by a Reset (the current MSHR
		// entry, if any, belongs to a newer request). Correlating by the
		// outstanding request's ID keeps a stale pre-reset translation from
		// filling the reset TLB and satisfying a fresh request.
		m.bottomPort().RetrieveIncoming()
		return true
	}

	// The downstream translation arrived for every request coalesced on this
	// MSHR entry: the translation wait (the time their downstream req_out was in
	// flight) is over for each req_in. network_busy is charged only later, when
	// the response is actually sent upstream in respondMSHREntry.
	for _, req := range next.MSHREntries[mshrIdx].Requests {
		tasks.AddMilestone(m.comp, tasks.Milestone{
			TaskID: req.RecvTaskID,
			Kind:   tasks.MilestoneKindTranslation,
			What:   "translation",
		})
	}

	setID := vAddrToSetID(page.VAddr, spec)
	wayID, ok := setEvict(&next.Sets[setID])

	if !ok {
		panic("failed to evict")
	}

	setUpdate(&next.Sets[setID], wayID, page)
	setVisit(&next.Sets[setID], wayID)

	next.HasRespondingMSHR = true
	next.RespondingMSHRData = next.MSHREntries[mshrIdx]
	next.RespondingMSHRData.Page = page

	// Get reqToBottom for tracing before removing
	sendTaskID := next.MSHREntries[mshrIdx].SendTaskID

	next.MSHREntries = mshrRemove(next.MSHREntries, page.PID, page.VAddr)
	m.bottomPort().RetrieveIncoming()

	if next.RespondingMSHRData.HasReqToBottom {
		tasks.EndReqOut(m.comp, sendTaskID)
	}

	return true
}
