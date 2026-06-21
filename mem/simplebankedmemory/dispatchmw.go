package simplebankedmemory

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type dispatchMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *dispatchMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *dispatchMW) Tick() bool {
	if m.comp.State.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}
	return m.dispatchFromTopPort()
}

func (m *dispatchMW) dispatchFromTopPort() bool {
	madeProgress := false
	spec := m.comp.Spec()
	next := &m.comp.State

	for {
		msgI := m.topPort().PeekIncoming()
		if msgI == nil {
			break
		}

		msg, ok := msgI.(memprotocol.AccessReq)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msgI)
		}

		if spec.NumBanks == 0 {
			log.Panic("simplebankedmemory: no banks configured")
		}

		bankID := selectBank(spec, bankSelectionAddress(spec, msg.GetAddress()))
		if bankID < 0 || bankID >= spec.NumBanks {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		if !pipelineCanAccept(next.Banks[bankID], spec) {
			break
		}

		// The selected bank's pipeline has room: the at-head wait the message
		// spent blocked on this bank's resources is over. This admission wait
		// belongs to the incoming-buffer task; req_in (opened at retrieve, below)
		// covers only the processing that follows.
		tracing.AddMilestone(m.comp, tracing.Milestone{
			TaskID: tracing.MsgIDAtIncomingBuffer(msgI, m.comp),
			Kind:   tracing.MilestoneKindHardwareResource,
			What:   fmt.Sprintf("%s.bank%d", m.comp.Name(), bankID),
		})

		m.topPort().RetrieveIncoming()

		// Admit the request: open req_in at retrieve, then open the pipeline
		// subtask as a child of req_in so the bank-pipeline latency is attributed
		// rather than left as a gap between the buffer task (which ends at
		// retrieve) and the post-pipeline finalize milestones on req_in. The task
		// ID rides on the item through the pipeline and is closed at finalize.
		tracing.TraceReqReceive(m.comp, msg)

		pipelineTaskID := timing.GetIDGenerator().Generate()
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:       pipelineTaskID,
			ParentID: tracing.MsgIDAtReceiver(msg, m.comp),
			Kind:     tracing.PipelineTaskKind,
			What:     m.comp.Name() + ".pipeline",
		})

		item := m.msgToItem(msg)
		item.PipelineTaskID = pipelineTaskID
		pipelineAccept(&next.Banks[bankID], spec, item)
		madeProgress = true
	}

	return madeProgress
}

func (m *dispatchMW) msgToItem(msg messaging.Msg) bankPipelineItemState {
	switch r := msg.(type) {
	case memprotocol.ReadReq:
		return bankPipelineItemState{
			IsRead:  true,
			ReadMsg: r,
		}
	case memprotocol.WriteReq:
		return bankPipelineItemState{
			IsRead:   false,
			WriteMsg: r,
		}
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", msg)
		return bankPipelineItemState{}
	}
}
