package mmuCache

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
		madeProgress = m.handleMMUCacheFlush(msg) || madeProgress
	case *RestartReq:
		madeProgress = m.handleMMUCacheRestart(msg) || madeProgress
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
	case mmuCacheStateEnable:
		if msg.Enable {
			log.Panic("mmuCache is already enabled")
		}
	case mmuCacheStatePause:
		if msg.Pause {
			log.Panic("mmuCache is already paused")
		}
		if msg.Drain {
			log.Panic("Cannot drain when mmuCache is paused")
		}
	case mmuCacheStateDrain:
		if msg.Drain {
			log.Panic("mmuCache is already draining")
		}
		if msg.Pause || msg.Enable {
			log.Panic("Cannot pause/enable when mmuCache is draining")
		}
	case mmuCacheStateFlush:
		if msg.Drain || msg.Enable || msg.Pause {
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

	req := item.(*mem.ControlMsg)

	if req.Enable {
		m.state = mmuCacheStateEnable
	} else if req.Drain {
		m.state = mmuCacheStateDrain
	} else if req.Pause {
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

func (m *ctrlMiddleware) handleMMUCacheFlush(req *FlushReq) bool {
	m.flushMsgMustBeValidInCurrentStage(req)
	m.inflightFlushReq = req
	m.controlPort.RetrieveIncoming()
	m.state = mmuCacheStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(req *FlushReq) {
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
		log.Panicf("Unknown mmuCache state: %s, msg: %s", state, reflect.TypeOf(req))
	}
}

func (m *ctrlMiddleware) handleMMUCacheRestart(req *RestartReq) bool {
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
