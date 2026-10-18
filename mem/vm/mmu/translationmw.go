package mmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// translationMW handles translation requests: parsing from top,
// page table walks, and sending responses for local hits.
type translationMW struct {
	comp *Comp
}

// Port helpers.

func (m *translationMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *translationMW) pageTable() *vm.PageTable {
	return m.comp.Resources.PageTable
}

// Handle runs the translation stages. Paused MMUs make no progress;
// draining MMUs continue walks but accept no new requests.
func (m *translationMW) Handle(_ timing.Event) bool {
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

func (m *translationMW) walkPageTable() bool {
	madeProgress := false

	state := &m.comp.State

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if state.WalkingTranslations[i].CycleLeft > 0 {
			state.WalkingTranslations[i].CycleLeft--
			madeProgress = true

			continue
		}

		madeProgress = m.finalizePageWalk(i) || madeProgress
	}

	tmp := state.WalkingTranslations[:0]

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if !m.toRemove(i) {
			tmp = append(tmp, state.WalkingTranslations[i])
		}
	}

	state.WalkingTranslations = tmp
	state.ToRemoveFromPTW = nil

	return madeProgress
}

func (m *translationMW) finalizePageWalk(walkingIndex int) bool {
	spec := m.comp.Spec
	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	page, found := m.pageTable().Find(
		vm.PID(walking.PID), walking.VAddr)

	if !found {
		if spec.AutoPageAllocation {
			page = m.createDefaultPage(
				vm.PID(walking.PID), walking.VAddr, walking.DeviceID)
			m.pageTable().Insert(page)
		} else {
			panic("page not found")
		}
	}

	state.WalkingTranslations[walkingIndex].Page = page

	return m.doPageWalkHit(walkingIndex)
}

func (m *translationMW) doPageWalkHit(walkingIndex int) bool {
	if !m.topPort().CanSend() {
		return false
	}

	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	rsp := messaging.Msg{Payload: vmprotocol.TranslationRsp{
		Page: walking.Page,
	},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          walking.ReqSrc,
		RspTo:        walking.ReqID,
		TrafficClass: "vmprotocol.TranslationRsp"}

	m.topPort().Send(rsp)
	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	// The page walk is fully local (a CycleLeft countdown, no downstream
	// req_out): label that latency interval as work on the req_in and close the
	// walk subtask that spanned it, so the work milestone has a corresponding
	// child bar instead of an unattributed gap. walking.RecvTaskID is the
	// req_in id.
	tasks.AddMilestone(m.comp, tasks.Milestone{
		TaskID: walking.RecvTaskID,
		Kind:   tasks.MilestoneKindWork,
		What:   m.comp.Name() + ".walk",
	})
	tasks.EndTask(m.comp, tasks.EndInfo{ID: walking.WalkTaskID})

	tasks.EndReqIn(m.comp, walking.RecvTaskID)

	return true
}

func (m *translationMW) parseFromTop() bool {
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
	m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".walk"))

	m.topPort().RetrieveIncoming()

	switch reqI.Payload.(type) {
	case vmprotocol.TranslationReq:
		req := reqI

		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s",
			fmt.Sprintf("%T", reqI.Payload))
	}

	return true
}

func (m *translationMW) startWalking(req messaging.Msg) {
	spec := m.comp.Spec
	state := &m.comp.State

	recvTaskID := tasks.StartReqIn(m.comp, req)

	ts := transactionState{
		ReqID:        req.ID,
		RecvTaskID:   recvTaskID,
		ReqSrc:       req.Src,
		ReqDst:       req.Dst,
		PID:          uint32(req.Payload.(vmprotocol.TranslationReq).PID),
		VAddr:        req.Payload.(vmprotocol.TranslationReq).VAddr,
		DeviceID:     req.Payload.(vmprotocol.TranslationReq).DeviceID,
		TransLatency: req.Payload.(vmprotocol.TranslationReq).TransLatency,
		CycleLeft:    spec.Latency,
	}

	// Open the walk subtask spanning the page-table-walk latency, a child of the
	// req_in, so the ".walk" work milestone emitted at walk completion has a
	// corresponding bar (no bare work interval).
	ts.WalkTaskID = tasks.StartTask(m.comp, tasks.StartInfo{
		ParentID: recvTaskID,
		Kind:     tasks.KindPipeline,
		What:     m.comp.Name() + ".walk",
	})

	state.WalkingTranslations = append(state.WalkingTranslations, ts)
}

func (m *translationMW) toRemove(index int) bool {
	state := &m.comp.State

	for i := 0; i < len(state.ToRemoveFromPTW); i++ {
		remove := state.ToRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}

	return false
}

func (m *translationMW) createDefaultPage(
	pid vm.PID, vAddr uint64, deviceID uint64,
) vm.Page {
	spec := m.comp.Spec
	alignedVAddr := (vAddr >> spec.Log2PageSize) << spec.Log2PageSize
	pageSize := uint64(1) << spec.Log2PageSize
	pAddr := m.allocatePhysicalPage()

	return vm.Page{
		PID:         pid,
		VAddr:       alignedVAddr,
		PAddr:       pAddr,
		PageSize:    pageSize,
		Valid:       true,
		DeviceID:    deviceID,
		Unified:     true,
		IsMigrating: false,
		IsPinned:    false,
	}
}

func (m *translationMW) allocatePhysicalPage() uint64 {
	spec := m.comp.Spec
	state := &m.comp.State
	pageSize := uint64(1) << spec.Log2PageSize

	for {
		candidatePage := (state.NextPhysicalPage >> spec.Log2PageSize) << spec.Log2PageSize

		if _, found := m.pageTable().ReverseLookup(candidatePage); !found {
			state.NextPhysicalPage = candidatePage + pageSize
			return candidatePage
		}

		state.NextPhysicalPage += pageSize
	}
}
