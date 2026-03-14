package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *ctrlMiddleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) controlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	madeProgress := false
	msgI := m.controlPort().PeekIncoming()

	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case *mem.ControlMsg:
		madeProgress = m.handleControlMsg(msg) || madeProgress
	case *tlb.FlushReq:
		madeProgress = m.handleMMUCacheFlush(msg) || madeProgress
	case *tlb.RestartReq:
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
	state := m.comp.GetState()
	switch s := state.CurrentState; s {
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
	itemI := m.controlPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	msg := itemI.(*mem.ControlMsg)
	next := m.comp.GetNextState()

	if msg.Enable {
		next.CurrentState = mmuCacheStateEnable
	} else if msg.Drain {
		next.CurrentState = mmuCacheStateDrain
	} else if msg.Pause {
		next.CurrentState = mmuCacheStatePause
	}

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	return true
}

func (m *ctrlMiddleware) handleMMUCacheFlush(msg *tlb.FlushReq) bool {
	m.flushMsgMustBeValidInCurrentStage(msg)

	next := m.comp.GetNextState()
	next.InflightFlushReqActive = true
	next.InflightFlushReqID = msg.ID
	next.InflightFlushReqSrc = msg.Src
	m.controlPort().RetrieveIncoming()
	next.CurrentState = mmuCacheStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg sim.Msg) {
	state := m.comp.GetState()
	switch s := state.CurrentState; s {
	case mmuCacheStateEnable:
		// valid
	case mmuCacheStatePause:
		log.Panic("Cannot flush when mmuCache is paused")
	case mmuCacheStateDrain:
		log.Panic("Cannot flush when mmuCache is draining")
	case mmuCacheStateFlush:
		log.Panic("mmuCache is already flushing")
	default:
		log.Panicf("Unknown mmuCache state: %s, msg: %s", s, reflect.TypeOf(msg))
	}
}

func (m *ctrlMiddleware) handleMMUCacheRestart(msg *tlb.RestartReq) bool {
	rsp := &tlb.RestartRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficClass = "mmuCache.RestartRsp"

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
	next.CurrentState = mmuCacheStateEnable

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()

	return true
}
