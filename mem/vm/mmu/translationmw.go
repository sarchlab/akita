package mmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
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
	if m.comp.State.ControlState == controlStatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.sendFaults() || madeProgress
	madeProgress = m.walkPageTable() || madeProgress
	if m.comp.State.ControlState == controlStateEnabled {
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

	if m.faults(walking, page) {
		return m.doPageWalkFault(walkingIndex)
	}

	return m.doPageWalkHit(walkingIndex)
}

// faults reports whether a walk faults: faults are enabled, and the page is
// unified, on another device than the requester, and not pinned.
func (m *translationMW) faults(walking transactionState, page vm.Page) bool {
	return m.comp.Spec.FaultHandler != "" && page.Unified &&
		page.DeviceID != walking.DeviceID && !page.IsPinned
}

// doPageWalkFault answers a faulted walk at once and queues the notification
// of the fault handler. The MMU never holds a walk while the fault is
// serviced (a held walk would wait for the migration, which waits for the
// requester to drain its translations): the requester holds the access and
// translates it again when it is released.
func (m *translationMW) doPageWalkFault(walkingIndex int) bool {
	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	if !m.doPageWalkHit(walkingIndex) {
		return false
	}

	state.PendingFaults = append(state.PendingFaults, messaging.Msg{
		Payload: addrtransprotocol.PageFault{
			PID:      vm.PID(walking.PID),
			VAddr:    walking.VAddr,
			DeviceID: walking.DeviceID,
			Page:     walking.Page,
		},
		ID:  m.comp.NewID(),
		Src: m.comp.Ports.Notification.AsRemote(),
		Dst: m.comp.Spec.FaultHandler,
	})
	state.NumFaults++

	return true
}

// sendFaults sends the queued fault notifications from the Notification
// port.
func (m *translationMW) sendFaults() bool {
	state := &m.comp.State
	madeProgress := false

	port := m.comp.Ports.Notification
	for len(state.PendingFaults) > 0 && port.CanSend() {
		port.Send(state.PendingFaults[0])
		state.PendingFaults = state.PendingFaults[1:]
		madeProgress = true
	}

	return madeProgress
}

func (m *translationMW) doPageWalkHit(walkingIndex int) bool {
	if !m.topPort().CanSend() {
		return false
	}

	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	rsp := messaging.Msg{Payload: addrtransprotocol.Rsp{
		Page:  walking.Page,
		Fault: m.faults(walking, walking.Page),
	},
		ID:    m.comp.NewID(),
		Src:   m.topPort().AsRemote(),
		Dst:   walking.ReqSrc,
		RspTo: walking.ReqID,
	}

	m.topPort().Send(rsp)
	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	if rsp.Payload.(addrtransprotocol.Rsp).Fault {
		m.comp.TaskTracker().AddTag(tasks.Tag{
			TaskID: walking.RecvTaskID,
			What:   "fault",
		})
	}

	// The page walk is fully local (a CycleLeft countdown, no downstream
	// req_out): label that latency interval as work on the req_in and close the
	// walk subtask that spanned it, so the work milestone has a corresponding
	// child bar instead of an unattributed gap. walking.RecvTaskID is the
	// req_in id.
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: walking.RecvTaskID,
		Kind:   tasks.MilestoneKindWork,
		What:   m.comp.Name() + ".walk",
	})
	m.comp.TaskTracker().EndTask(walking.WalkTaskID)

	m.comp.TaskTracker().EndReqIn(walking.RecvTaskID)

	return true
}

func (m *translationMW) parseFromTop() bool {
	spec := m.comp.Spec
	state := &m.comp.State

	reqI, ok := m.topPort().PeekIncoming()
	if !ok {
		return false
	}

	if len(state.WalkingTranslations) >= spec.MaxInFlightTranslations {
		return false
	}

	// A walk slot is free: the at-head wait for one — counted on the
	// incoming-buffer task since the request reached the head of the Top
	// buffer — is over. The req_in opened at retrieve covers only the walk
	// processing that follows.
	m.topPort().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".walk"))

	m.topPort().RetrieveIncoming()

	switch reqI.Payload.(type) {
	case addrtransprotocol.Req:
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

	recvTaskID := m.comp.TaskTracker().StartReqIn(m.comp, req)

	ts := transactionState{
		ReqID:        req.ID,
		RecvTaskID:   recvTaskID,
		ReqSrc:       req.Src,
		ReqDst:       req.Dst,
		PID:          uint32(req.Payload.(addrtransprotocol.Req).PID),
		VAddr:        req.Payload.(addrtransprotocol.Req).VAddr,
		DeviceID:     req.Payload.(addrtransprotocol.Req).DeviceID,
		TransLatency: req.Payload.(addrtransprotocol.Req).TransLatency,
		CycleLeft:    spec.Latency,
	}

	// Open the walk subtask spanning the page-table-walk latency, a child of the
	// req_in, so the ".walk" work milestone emitted at walk completion has a
	// corresponding bar (no bare work interval).
	ts.WalkTaskID = m.comp.TaskTracker().StartTask(m.comp, tasks.StartInfo{
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
