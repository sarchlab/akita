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
		for {
			progress := m.finalizeSingle(&state.Banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

// finalizeSingle completes the bank's oldest ready request. A request whose
// response cannot be sent stays in its lane's final stage, already committed.
func (m *tickFinalizeMW) finalizeSingle(b *bankState) bool {
	lane, ok := b.Pipeline.ReadyLane()
	if !ok {
		return false
	}
	item, _ := b.Pipeline.PeekLane(lane)

	if item.IsRead {
		return m.finalizeRead(b, lane, &item)
	}

	return m.finalizeWrite(b, lane, &item)
}

func (m *tickFinalizeMW) finalizeRead(
	b *bankState,
	lane int,
	item *bankPipelineItemState,
) bool {
	readReq := item.ReadMsg

	if !item.Committed {
		data := m.comp.Resources.Storage.Read(
			readReq.Payload.(memprotocol.ReadReq).Address, readReq.Payload.(memprotocol.ReadReq).AccessByteSize)

		item.ReadData = data
		item.Committed = true

		// Keep the committed state with the item while it waits to respond.
		b.Pipeline.UpdateLane(lane, *item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	// The item is leaving the bank pipeline (it is ready at the head of its
	// lane and about to be popped). Attribute the pipeline
	// traversal as work on req_in before the response send and before
	// EndReqIn, so the same-tick complete does not absorb it via the
	// same-time dedup.
	m.finishPipeline(item.RecvTaskID, item.PipelineTaskID)

	rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{
		Data: item.ReadData},
		ID:    m.comp.NewID(),
		Src:   m.topPort().AsRemote(),
		Dst:   readReq.Src,
		RspTo: readReq.ID,

		TrafficBytes: len(item.ReadData) + 4,
	}

	m.topPort().Send(rsp)

	m.comp.TaskTracker().EndReqIn(item.RecvTaskID)

	b.Pipeline.PopLane(lane)

	return true
}

func (m *tickFinalizeMW) finalizeWrite(
	b *bankState,
	lane int,
	item *bankPipelineItemState,
) bool {
	writeReq := item.WriteMsg

	if !item.Committed {
		addr := writeReq.Payload.(memprotocol.WriteReq).Address

		if writeReq.Payload.(memprotocol.WriteReq).DirtyMask == nil {
			m.comp.Resources.Storage.Write(addr, writeReq.Payload.(memprotocol.WriteReq).Data)
		} else {
			data := m.comp.Resources.Storage.Read(addr, uint64(len(writeReq.Payload.(memprotocol.WriteReq).Data)))

			for i := range writeReq.Payload.(memprotocol.WriteReq).Data {
				if writeReq.Payload.(memprotocol.WriteReq).DirtyMask[i] {
					data[i] = writeReq.Payload.(memprotocol.WriteReq).Data[i]
				}
			}

			m.comp.Resources.Storage.Write(addr, data)
		}

		item.Committed = true
		b.Pipeline.UpdateLane(lane, *item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	// See finalizeRead: attribute the bank-pipeline traversal as work on
	// req_in at pipeline exit, before the response send and EndReqIn.
	m.finishPipeline(item.RecvTaskID, item.PipelineTaskID)

	rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          writeReq.Src,
		RspTo:        writeReq.ID,
		TrafficBytes: 4,
	}

	m.topPort().Send(rsp)

	m.comp.TaskTracker().EndReqIn(item.RecvTaskID)

	b.Pipeline.PopLane(lane)

	return true
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
