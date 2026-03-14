package tlb

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *ctrlMiddleware) controlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	madeProgress := false
	msg := m.controlPort().PeekIncoming()

	if msg == nil {
		return false
	}

	switch msg.(type) {
	case *mem.ControlMsg:
		madeProgress = m.handleControlMsg(msg.(*mem.ControlMsg)) || madeProgress
	case *FlushReq:
		madeProgress = m.handleTLBFlush(msg.(*FlushReq)) || madeProgress
	case *RestartReq:
		madeProgress = m.handleTLBRestart(msg.(*RestartReq)) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(msg *mem.ControlMsg) bool {
	m.ctrlMsgMustBeValidInCurrentStage(msg)

	return m.performCtrlReq()
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidInCurrentStage(
	ctrlMsg *mem.ControlMsg,
) {
	state := m.comp.GetState()
	switch s := state.TLBState; s {
	case tlbStateEnable:
		if ctrlMsg.Enable {
			log.Panic("TLB is already enabled")
		}
	case tlbStatePause:
		if ctrlMsg.Pause {
			log.Panic("TLB is already paused")
		}
		if ctrlMsg.Drain {
			log.Panic("Cannot drain when TLB is paused")
		}
	case tlbStateDrain:
		if ctrlMsg.Drain {
			log.Panic("TLB is already draining")
		}
		if ctrlMsg.Pause || ctrlMsg.Enable {
			log.Panic("Cannot pause/enable when TLB is draining")
		}
	case tlbStateFlush:
		if ctrlMsg.Drain || ctrlMsg.Enable || ctrlMsg.Pause {
			log.Panic("Cannot pause/enable/drain when TLB is flushing")
		}
	default:
		log.Panic("Unknown TLB state")
	}
}

func (m *ctrlMiddleware) performCtrlReq() bool {
	itemI := m.controlPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(*mem.ControlMsg)
	next := m.comp.GetNextState()

	if item.Enable {
		next.TLBState = tlbStateEnable
	} else if item.Drain {
		next.TLBState = tlbStateDrain
	} else if item.Pause {
		next.TLBState = tlbStatePause
	}

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	return true
}

func (m *ctrlMiddleware) handleTLBFlush(msg *FlushReq) bool {
	m.flushMsgMustBeValidInCurrentStage(msg)

	next := m.comp.GetNextState()
	next.HasInflightFlushReq = true
	next.InflightFlushReqMsg = *msg
	m.controlPort().RetrieveIncoming()
	next.TLBState = tlbStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg sim.Msg) {
	state := m.comp.GetState()
	switch s := state.TLBState; s {
	case tlbStateEnable:
		// valid
	case tlbStatePause:
		log.Panic("Cannot flush when TLB is paused")
	case tlbStateDrain:
		log.Panic("Cannot flush when TLB is draining")
	case tlbStateFlush:
		log.Panic("TLB is already flushing")
	default:
		log.Panicf("Unknown TLB state: %s, msg: %s", s, reflect.TypeOf(msg))
	}
}

func (m *ctrlMiddleware) handleTLBRestart(msg *RestartReq) bool {
	rsp := &RestartRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficClass = "tlb.RestartRsp"

	err := m.controlPort().Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	next := m.comp.GetNextState()
	next.TLBState = tlbStateEnable

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()

	return true
}
