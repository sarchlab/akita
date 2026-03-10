package tlb

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	*Comp
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	madeProgress := false
	msg := m.controlPort.PeekIncoming()

	if msg == nil {
		return false
	}

	switch msg.Payload.(type) {
	case *mem.ControlMsgPayload:
		madeProgress = m.handleControlMsg(msg) || madeProgress
	case *FlushReqPayload:
		madeProgress = m.handleTLBFlush(msg) || madeProgress
	case *RestartReqPayload:
		madeProgress = m.handleTLBRestart(msg) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(msg *sim.Msg) bool {
	ctrlPayload := sim.MsgPayload[mem.ControlMsgPayload](msg)
	m.ctrlMsgMustBeValidInCurrentStage(ctrlPayload)

	return m.performCtrlReq()
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidInCurrentStage(
	ctrlPayload *mem.ControlMsgPayload,
) {
	switch state := m.state; state {
	case tlbStateEnable:
		if ctrlPayload.Enable {
			log.Panic("TLB is already enabled")
		}
	case tlbStatePause:
		if ctrlPayload.Pause {
			log.Panic("TLB is already paused")
		}
		if ctrlPayload.Drain {
			log.Panic("Cannot drain when TLB is paused")
		}
	case tlbStateDrain:
		if ctrlPayload.Drain {
			log.Panic("TLB is already draining")
		}
		if ctrlPayload.Pause || ctrlPayload.Enable {
			log.Panic("Cannot pause/enable when TLB is draining")
		}
	case tlbStateFlush:
		if ctrlPayload.Drain || ctrlPayload.Enable || ctrlPayload.Pause {
			log.Panic("Cannot pause/enable/drain when TLB is flushing")
		}
	default:
		log.Panic("Unknown TLB state")
	}
}

func (m *ctrlMiddleware) performCtrlReq() bool {
	item := m.controlPort.PeekIncoming()
	if item == nil {
		return false
	}

	ctrlPayload := sim.MsgPayload[mem.ControlMsgPayload](item)

	if ctrlPayload.Enable {
		m.state = tlbStateEnable
	} else if ctrlPayload.Drain {
		m.state = tlbStateDrain
	} else if ctrlPayload.Pause {
		m.state = tlbStatePause
	}

	m.controlPort.RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	return true
}

func (m *ctrlMiddleware) handleTLBFlush(msg *sim.Msg) bool {
	m.flushMsgMustBeValidInCurrentStage(msg)
	m.inflightFlushReq = msg
	m.controlPort.RetrieveIncoming()
	m.state = tlbStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg *sim.Msg) {
	switch state := m.state; state {
	case tlbStateEnable:
		// valid
	case tlbStatePause:
		log.Panic("Cannot flush when TLB is paused")
	case tlbStateDrain:
		log.Panic("Cannot flush when TLB is draining")
	case tlbStateFlush:
		log.Panic("TLB is already flushing")
	default:
		log.Panicf("Unknown TLB state: %s, msg: %s", state, reflect.TypeOf(msg.Payload))
	}
}

func (m *ctrlMiddleware) handleTLBRestart(msg *sim.Msg) bool {
	rsp := RestartRspBuilder{}.
		WithSrc(m.controlPort.AsRemote()).
		WithDst(msg.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	m.state = tlbStateEnable

	for m.topPort.PeekIncoming() != nil {
		m.topPort.RetrieveIncoming()
	}

	for m.bottomPort.PeekIncoming() != nil {
		m.bottomPort.RetrieveIncoming()
	}

	m.controlPort.RetrieveIncoming()

	return true
}
