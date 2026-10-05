package gmmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// walkMW handles the top→page-table walk path:
// parseFromTop, startWalking, walkPageTable, removeCompletedTranslations,
// processRemoteMemReq, finalizePageWalk, doPageWalkHit.
type walkMW struct {
	comp *Comp

	// pageTable is Resources.PageTable, taken by newMiddlewares.
	pageTable vm.PageTable
}

func (m *walkMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *walkMW) bottomPort() messaging.Port {
	return m.comp.Ports.Bottom
}

// Handle runs the walk stages. Paused GMMUs make no progress; draining
// GMMUs continue page-table walks but accept no new requests.
func (m *walkMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		madeProgress = m.parseFromTop() || madeProgress
	}

	return madeProgress
}

func (m *walkMW) parseFromTop() bool {
	spec := m.comp.Spec
	state := &m.comp.State

	reqI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}

	if len(state.WalkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	// A walk slot is free: the at-head wait for one — counted on the
	// incoming-buffer task since the request reached the head of the Top
	// buffer — is over. The req_in opened at retrieve covers only the walk
	// processing that follows.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(reqI, m.comp),
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   m.comp.Name() + ".walk",
	})

	m.topPort().RetrieveIncoming()

	switch reqI.Payload.(type) {
	case vmprotocol.TranslationReq:
		req := reqI

		tracing.TraceReqReceive(m.comp, req)
		m.startWalking(req)
	default:
		log.Panicf("gmmu cannot handle request of type %s",
			fmt.Sprintf("%T", reqI.Payload))
	}

	return true
}

func (m *walkMW) startWalking(req messaging.Msg) {
	spec := m.comp.Spec
	state := &m.comp.State

	recvTaskID := tracing.MsgIDAtReceiver(req, m.comp)
	walkTaskID := m.comp.NewID()

	ts := transactionState{
		ReqID:      req.ID,
		RecvTaskID: recvTaskID,
		ReqSrc:     req.Src,
		ReqDst:     req.Dst,
		PID:        uint64(req.Payload.(vmprotocol.TranslationReq).PID),
		VAddr:      req.Payload.(vmprotocol.TranslationReq).VAddr,
		DeviceID:   req.Payload.(vmprotocol.TranslationReq).DeviceID,
		CycleLeft:  spec.Latency,
		WalkTaskID: walkTaskID,
	}

	// Open the walk subtask spanning the page-table-walk latency, a child of the
	// req_in, so the ".walk" work milestone has a corresponding bar. It is closed
	// on the local-hit (doPageWalkHit) and remote-fetch (processRemoteMemReq)
	// exits.
	tracing.StartTask(m.comp, tracing.TaskStart{
		ID:       walkTaskID,
		ParentID: recvTaskID,
		Kind:     tracing.PipelineTaskKind,
		What:     m.comp.Name() + ".walk",
	})

	state.WalkingTranslations = append(
		state.WalkingTranslations, ts)
}

func (m *walkMW) walkPageTable() bool {
	state := &m.comp.State

	if len(state.WalkingTranslations) == 0 {
		return false
	}

	madeProgress := false
	spec := m.comp.Spec

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if state.WalkingTranslations[i].CycleLeft > 0 {
			state.WalkingTranslations[i].CycleLeft--
			madeProgress = true
			continue
		}

		ts := state.WalkingTranslations[i]

		page, found := m.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
		if !found {
			log.Panicf(
				"gmmu: page not found for PID %d VAddr 0x%x",
				ts.PID, ts.VAddr,
			)
		}

		if page.DeviceID == spec.DeviceID {
			madeProgress = m.finalizePageWalk(state, i) || madeProgress
		} else {
			madeProgress = m.processRemoteMemReq(state, i) || madeProgress
		}
	}

	m.removeCompletedTranslations(state)

	return madeProgress
}

func (m *walkMW) removeCompletedTranslations(state *state) {
	if len(state.ToRemoveFromPTW) == 0 {
		return
	}

	toRemoveSet := make(map[int]bool, len(state.ToRemoveFromPTW))
	for _, idx := range state.ToRemoveFromPTW {
		toRemoveSet[idx] = true
	}

	tmp := state.WalkingTranslations[:0]
	for i := 0; i < len(state.WalkingTranslations); i++ {
		if !toRemoveSet[i] {
			tmp = append(tmp, state.WalkingTranslations[i])
		}
	}
	state.WalkingTranslations = tmp
	state.ToRemoveFromPTW = nil
}

func (m *walkMW) processRemoteMemReq(
	state *state,
	walkingIndex int,
) bool {
	if !m.bottomPort().CanSend() {
		return false
	}

	spec := m.comp.Spec
	walking := state.WalkingTranslations[walkingIndex]

	req := messaging.Msg{Payload: vmprotocol.TranslationReq{
		PID:      vm.PID(walking.PID),
		VAddr:    walking.VAddr,
		DeviceID: walking.DeviceID},
		ID:  m.comp.NewID(),
		Src: m.bottomPort().AsRemote(),
		Dst: spec.LowModule,

		TrafficClass: "vmprotocol.TranslationReq"}

	state.RemoteMemReqs[req.ID] = walking

	m.bottomPort().Send(req)

	// Open the downstream req_out as a child task of the original req_in so the
	// remote walk shows up as a proper subtask; it is finalized in
	// handleTranslationRsp when the remote response returns.
	tracing.TraceReqInitiate(m.comp, req, walking.RecvTaskID)

	// The local walk countdown finished and the page is remote: attribute the
	// countdown as work on the req_in and close the walk subtask that spanned
	// it. The downstream req_out (opened above) then covers the remote fetch.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: walking.RecvTaskID,
		Kind:   tracing.MilestoneKindWork,
		What:   m.comp.Name() + ".walk",
	})
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: walking.WalkTaskID})

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	return true
}

func (m *walkMW) finalizePageWalk(
	state *state,
	walkingIndex int,
) bool {
	ts := state.WalkingTranslations[walkingIndex]
	page, found := m.pageTable.Find(vm.PID(ts.PID), ts.VAddr)
	if !found {
		return false
	}

	state.WalkingTranslations[walkingIndex].Page = pageStateFromPage(page)

	return m.doPageWalkHit(state, walkingIndex)
}

func (m *walkMW) doPageWalkHit(
	state *state,
	walkingIndex int,
) bool {
	if !m.topPort().CanSend() {
		return false
	}
	walking := state.WalkingTranslations[walkingIndex]

	rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
		Page: pageFromPageState(walking.Page),
	},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          walking.ReqSrc,
		RspTo:        walking.ReqID,
		TrafficClass: "vmprotocol.TranslationRsp"}

	m.topPort().Send(rsp)

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	// The local page walk counted down CycleLeft before reaching here; record
	// that processing latency as work on the original req_in and close the walk
	// subtask that spanned it, so the work milestone has a corresponding bar.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: walking.RecvTaskID,
		Kind:   tracing.MilestoneKindWork,
		What:   m.comp.Name() + ".walk",
	})
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: walking.WalkTaskID})

	tracing.TraceReqComplete(
		m.comp, messaging.Msg{ID: walking.ReqID,
			Src: walking.ReqSrc,
			Dst: walking.ReqDst, Payload: vmprotocol.TranslationReq{}},
	)

	return true
}
