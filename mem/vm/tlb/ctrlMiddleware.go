package tlb

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/tracing"
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

	switch msg := msg.(type) {
	case *mem.ControlMsg:
		madeProgress = m.handleControlMsg(msg) || madeProgress
	case *FlushReq:
		madeProgress = m.handleTLBFlush(msg) || madeProgress
	case *RestartReq:
		madeProgress = m.handleTLBRestart(msg) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(
	msg *mem.ControlMsg) bool {
	m.ctrlMsgMustBeValidInCurrentStage(msg)

	return m.performCtrlReq()
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidInCurrentStage(msg *mem.ControlMsg) {
	switch state := m.state; state {
	case tlbStateEnable:
		if msg.Enable {
			log.Panic("TLB is already enabled")
		}
	case tlbStatePause:
		if msg.Pause {
			log.Panic("TLB is already paused")
		}
		if msg.Drain {
			log.Panic("Cannot drain when TLB is paused")
		}
	case tlbStateDrain:
		if msg.Drain {
			log.Panic("TLB is already draining")
		}
		if msg.Pause || msg.Enable {
			log.Panic("Cannot pause/enable when TLB is draining")
		}
	case tlbStateFlush:
		if msg.Drain || msg.Enable || msg.Pause {
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

	req := item.(*mem.ControlMsg)

	if req.Enable {
		m.state = tlbStateEnable
	} else if req.Drain {
		m.state = tlbStateDrain
	} else if req.Pause {
		m.state = tlbStatePause
	}

	item = m.controlPort.RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	return true
}

func (m *ctrlMiddleware) handleTLBFlush(req *FlushReq) bool {
	m.flushMsgMustBeValidInCurrentStage(req)
	m.inflightFlushReq = req
	m.controlPort.RetrieveIncoming()
	m.state = tlbStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(req *FlushReq) {
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
		log.Panicf("Unknown TLB state: %s, msg: %s", state, reflect.TypeOf(req))
	}
}

func (m *ctrlMiddleware) handleTLBRestart(req *RestartReq) bool {
	rsp := RestartRspBuilder{}.
		WithSrc(m.controlPort.AsRemote()).
		WithDst(req.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
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
