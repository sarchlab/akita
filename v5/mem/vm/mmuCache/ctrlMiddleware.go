package mmuCache

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

	switch payload := msg.Payload.(type) {
	case *mem.ControlMsgPayload:
		madeProgress = m.handleControlMsg(msg, payload) || madeProgress
	case *FlushReqPayload:
		madeProgress = m.handleMMUCacheFlush(msg, payload) || madeProgress
	case *RestartReqPayload:
		madeProgress = m.handleMMUCacheRestart(msg) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(
	msg *sim.Msg, payload *mem.ControlMsgPayload) bool {
	m.ctrlMsgMustBeValidInCurrentStage(payload)

	return m.performCtrlReq()
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidInCurrentStage(payload *mem.ControlMsgPayload) {
	switch state := m.state; state {
	case mmuCacheStateEnable:
		if payload.Enable {
			log.Panic("mmuCache is already enabled")
		}
	case mmuCacheStatePause:
		if payload.Pause {
			log.Panic("mmuCache is already paused")
		}
		if payload.Drain {
			log.Panic("Cannot drain when mmuCache is paused")
		}
	case mmuCacheStateDrain:
		if payload.Drain {
			log.Panic("mmuCache is already draining")
		}
		if payload.Pause || payload.Enable {
			log.Panic("Cannot pause/enable when mmuCache is draining")
		}
	case mmuCacheStateFlush:
		if payload.Drain || payload.Enable || payload.Pause {
			log.Panic("Cannot pause/enable/drain when mmuCache is flushing")
		}
	default:
		log.Panic("Unknown mmuCache state")
	}
}

func (m *ctrlMiddleware) performCtrlReq() bool {
	item := m.controlPort.PeekIncoming()
	if item == nil {
		return false
	}

	payload := sim.MsgPayload[mem.ControlMsgPayload](item)

	if payload.Enable {
		m.state = mmuCacheStateEnable
	} else if payload.Drain {
		m.state = mmuCacheStateDrain
	} else if payload.Pause {
		m.state = mmuCacheStatePause
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

func (m *ctrlMiddleware) handleMMUCacheFlush(msg *sim.Msg, payload *FlushReqPayload) bool {
	m.flushMsgMustBeValidInCurrentStage(msg)
	m.inflightFlushReq = msg
	m.controlPort.RetrieveIncoming()
	m.state = mmuCacheStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg *sim.Msg) {
	switch state := m.state; state {
	case mmuCacheStateEnable:
		// valid
	case mmuCacheStatePause:
		log.Panic("Cannot flush when mmuCache is paused")
	case mmuCacheStateDrain:
		log.Panic("Cannot flush when mmuCache is draining")
	case mmuCacheStateFlush:
		log.Panic("mmuCache is already flushing")
	default:
		log.Panicf("Unknown mmuCache state: %s, msg: %s", state, reflect.TypeOf(msg.Payload))
	}
}

func (m *ctrlMiddleware) handleMMUCacheRestart(msg *sim.Msg) bool {
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

	m.state = mmuCacheStateEnable

	for m.topPort.PeekIncoming() != nil {
		m.topPort.RetrieveIncoming()
	}

	for m.bottomPort.PeekIncoming() != nil {
		m.bottomPort.RetrieveIncoming()
	}

	m.controlPort.RetrieveIncoming()

	return true
}
