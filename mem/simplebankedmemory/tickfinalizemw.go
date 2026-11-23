package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type tickFinalizeMW struct {
	comp *Comp
}

func (m *tickFinalizeMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *tickFinalizeMW) Handle(e timing.Event) bool {
	if m.comp.State.ControlState == controlStatePaused {
		return false
	}

	cycle := m.comp.Spec.Freq.Cycle(e.Time())
	for i := range m.comp.State.Banks {
		m.comp.State.Banks[i].Pipeline.AdvanceTo(cycle)
	}

	return m.finalizeBanks()
}

func (m *tickFinalizeMW) finalizeBanks() bool {
	madeProgress := false
	state := &m.comp.State

	for i := range state.Banks {
		b := &state.Banks[i]

		// Storage effects do not depend on response-port availability: every
		// ready lane head commits, oldest first, before any response is sent.
		b.Pipeline.UpdateReady(m.commit)

		for m.respondOldest(b) {
			madeProgress = true
		}
	}

	return madeProgress
}

// commit performs a ready request's storage access once. The result stays with
// the item in its lane's final stage until its response is sent.
func (m *tickFinalizeMW) commit(item bankPipelineItemState) bankPipelineItemState {
	if item.Committed {
		return item
	}

	if item.IsRead {
		req := item.ReadMsg.Payload.(memprotocol.ReadReq)
		item.ReadData = m.comp.Resources.Storage.Read(req.Address, req.AccessByteSize)
	} else {
		m.commitWrite(item.WriteMsg.Payload.(memprotocol.WriteReq))
	}
	item.Committed = true

	return item
}

func (m *tickFinalizeMW) commitWrite(req memprotocol.WriteReq) {
	if req.DirtyMask == nil {
		m.comp.Resources.Storage.Write(req.Address, req.Data)
		return
	}

	data := m.comp.Resources.Storage.Read(req.Address, uint64(len(req.Data)))
	for i := range req.Data {
		if req.DirtyMask[i] {
			data[i] = req.Data[i]
		}
	}

	m.comp.Resources.Storage.Write(req.Address, data)
}

// respondOldest sends the response of the bank's oldest ready request and
// removes it from its lane. A request whose response cannot be sent stays in
// its lane's final stage.
func (m *tickFinalizeMW) respondOldest(b *bankState) bool {
	if !m.topPort().CanSend() {
		return false
	}

	lane, ok := b.Pipeline.ReadyLane()
	if !ok {
		return false
	}
	item, _ := b.Pipeline.PeekLane(lane)

	// The item is leaving the bank pipeline (it is ready at the head of its
	// lane and about to be popped). Attribute the pipeline
	// traversal as work on req_in before the response send and before
	// EndReqIn, so the same-tick complete does not absorb it via the
	// same-time dedup.
	m.finishPipeline(item.RecvTaskID, item.PipelineTaskID)

	m.topPort().Send(m.makeResponse(&item))

	m.comp.TaskTracker().EndReqIn(item.RecvTaskID)

	b.Pipeline.PopLane(lane)

	return true
}

func (m *tickFinalizeMW) makeResponse(item *bankPipelineItemState) messaging.Msg {
	if item.IsRead {
		return messaging.Msg{Payload: memprotocol.DataReadyRsp{
			Data: item.ReadData},
			ID:    m.comp.NewID(),
			Src:   m.topPort().AsRemote(),
			Dst:   item.ReadMsg.Src,
			RspTo: item.ReadMsg.ID,

			TrafficBytes: len(item.ReadData) + 4,
		}
	}

	return messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          item.WriteMsg.Src,
		RspTo:        item.WriteMsg.ID,
		TrafficBytes: 4,
	}
}

// finishPipeline attributes the bank-pipeline traversal to the request's req_in
// task at pipeline exit. It emits a work milestone (the must-have: it marks the
// pipeline interval as productive work rather than a blocking gap) and closes
// the PipelineTaskKind subtask opened at dispatch. Call it before the response
// send and before EndReqIn so the same-tick complete does not shadow the
// work milestone via the dedup. Both task IDs are retained in the pipeline
// entry from dispatch through completion.
func (m *tickFinalizeMW) finishPipeline(recvTaskID, pipelineTaskID uint64) {
	m.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: recvTaskID,
		Kind:   tasks.MilestoneKindWork,
		What:   m.comp.Name() + ".pipeline",
	})

	if pipelineTaskID != 0 {
		m.comp.TaskTracker().EndTask(pipelineTaskID)
	}
}
