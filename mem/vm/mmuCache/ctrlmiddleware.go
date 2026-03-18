package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
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
	msgI := m.controlPort().PeekIncoming()

	if msgI == nil {
		return false
	}

	switch msg := msgI.(type) {
	case *mem.ControlReq:
		return m.handleControlReq(msg)
	default:
		log.Panicf("Unhandled message type: %s", reflect.TypeOf(msgI))
	}

	return false
}

func (m *ctrlMiddleware) handleControlReq(msg *mem.ControlReq) bool {
	switch msg.Command {
	case mem.CmdEnable:
		return m.performCtrlEnable(msg)
	case mem.CmdDrain:
		return m.performCtrlDrain(msg)
	case mem.CmdPause:
		return m.performCtrlPause(msg)
	case mem.CmdFlush:
		return m.handleMMUCacheFlush(msg)
	case mem.CmdReset:
		return m.handleMMUCacheRestart(msg)
	default:
		log.Panicf("Unhandled control command: %d", msg.Command)
	}

	return false
}

func (m *ctrlMiddleware) performCtrlEnable(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.CurrentState = mmuCacheStateEnable

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

func (m *ctrlMiddleware) performCtrlDrain(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.CurrentState = mmuCacheStateDrain

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

func (m *ctrlMiddleware) performCtrlPause(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.CurrentState = mmuCacheStatePause

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

func (m *ctrlMiddleware) handleMMUCacheFlush(msg *mem.ControlReq) bool {
	m.flushMsgMustBeValidInCurrentStage(msg)

	state := m.comp.GetNextState()
	state.InflightFlushReqActive = true
	state.InflightFlushReqID = msg.ID
	state.InflightFlushReqSrc = msg.Src
	m.controlPort().RetrieveIncoming()
	state.CurrentState = mmuCacheStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg sim.Msg) {
	state := m.comp.GetNextState()
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

func (m *ctrlMiddleware) handleMMUCacheRestart(msg *mem.ControlReq) bool {
	rsp := &mem.ControlRsp{Command: mem.CmdReset, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficClass = "mem.ControlRsp"

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

	state := m.comp.GetNextState()
	state.CurrentState = mmuCacheStateEnable

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()

	return true
}
