package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type tickFinalizeMW struct {
	comp *Comp
}

func (m *tickFinalizeMW) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *tickFinalizeMW) Handle(_ timing.Event) bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}
	madeProgress := m.finalizeBanks()
	madeProgress = m.tickPipelines() || madeProgress
	return madeProgress
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

func (m *tickFinalizeMW) finalizeSingle(b *bankState) bool {
	item, ok := bufferPeek(*b)
	if !ok {
		return false
	}

	if item.IsRead {
		return m.finalizeRead(b, &item)
	}

	return m.finalizeWrite(b, &item)
}

func (m *tickFinalizeMW) finalizeRead(
	b *bankState,
	item *bankPipelineItemState,
) bool {
	readReq := item.ReadMsg

	if !item.Committed {
		data := m.comp.Resources.Storage.Read(
			readReq.Payload.(memprotocol.ReadReq).Address, readReq.Payload.(memprotocol.ReadReq).AccessByteSize)

		item.ReadData = data
		item.Committed = true

		// Update the buffer head with the committed state.
		b.PostPipelineBuf.UpdateFront(*item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	// The item has just left the bank pipeline (it is at the head of
	// PostPipelineBuf and about to be dequeued). Attribute the pipeline
	// traversal as work on req_in before the response send and before
	// TraceReqComplete, so the same-tick complete does not absorb it via the
	// same-time dedup.
	m.finishPipeline(item.ReadMsg, item.PipelineTaskID)

	rsp := messaging.Msg{Payload: memprotocol.DataReadyRsp{
		Data: item.ReadData},
		ID:    m.comp.NewID(),
		Src:   m.topPort().AsRemote(),
		Dst:   readReq.Src,
		RspTo: readReq.ID,

		TrafficBytes: len(item.ReadData) + 4,
		TrafficClass: "memprotocol.DataReadyRsp"}

	m.topPort().Send(rsp)

	tracing.TraceReqComplete(m.comp, item.ReadMsg)

	bufferPop(b)

	return true
}

func (m *tickFinalizeMW) finalizeWrite(
	b *bankState,
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
		b.PostPipelineBuf.UpdateFront(*item)
	}

	if !m.topPort().CanSend() {
		return false
	}

	// See finalizeRead: attribute the bank-pipeline traversal as work on
	// req_in at pipeline exit, before the response send and TraceReqComplete.
	m.finishPipeline(item.WriteMsg, item.PipelineTaskID)

	rsp := messaging.Msg{Payload: memprotocol.WriteDoneRsp{},
		ID:           m.comp.NewID(),
		Src:          m.topPort().AsRemote(),
		Dst:          writeReq.Src,
		RspTo:        writeReq.ID,
		TrafficBytes: 4,
		TrafficClass: "memprotocol.WriteDoneRsp"}

	m.topPort().Send(rsp)

	tracing.TraceReqComplete(m.comp, item.WriteMsg)

	bufferPop(b)

	return true
}

// finishPipeline attributes the bank-pipeline traversal to the request's req_in
// task at pipeline exit. It emits a work milestone (the must-have: it marks the
// pipeline interval as productive work rather than a blocking gap) and closes
// the PipelineTaskKind subtask opened at dispatch. Call it before the response
// send and before TraceReqComplete so the same-tick complete does not shadow the
// work milestone via the dedup. req is the original request message (the req_in
// key, matching TraceReqReceive/TraceReqComplete); pipelineTaskID is the subtask
// opened at dispatch, or zero when tracing was disabled at dispatch.
func (m *tickFinalizeMW) finishPipeline(req messaging.Msg, pipelineTaskID uint64) {
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(req, m.comp),
		Kind:   tracing.MilestoneKindWork,
		What:   m.comp.Name() + ".pipeline",
	})

	if pipelineTaskID != 0 {
		tracing.EndTask(m.comp, tracing.TaskEnd{ID: pipelineTaskID})
	}
}

func (m *tickFinalizeMW) tickPipelines() bool {
	madeProgress := false
	state := &m.comp.State

	for i := range state.Banks {
		madeProgress = pipelineTick(&state.Banks[i]) || madeProgress
	}

	return madeProgress
}
