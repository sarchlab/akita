package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) controlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

// completePendingDrain detects that the data-path middleware finished
// draining (state transitioned to paused) and sends the async Drain
// Rsp.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if !state.PendingDrainRsp {
		return false
	}
	if state.CurrentState != mmuCacheStatePause {
		return false
	}
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	return true
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
		// TODO(control-protocol): Phase 3 renames this verb to
		// CmdInvalidate (it filters by Addresses+PID). For Phase 2 the
		// behavior stays under CmdFlush.
		return m.handleMMUCacheFlush(msg)
	case mem.CmdReset:
		return m.handleReset(msg)
	case mem.CmdInvalidate:
		return m.handleUnsupported(msg)
	default:
		return m.handleUnsupported(msg)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdEnable,
		msg.Src, msg.ID, true, ""))
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
	state := &m.comp.State
	state.CurrentState = mmuCacheStateDrain
	state.PendingDrainRsp = true
	state.CurrentCmdID = msg.ID
	state.CurrentCmdSrc = msg.Src

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
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.CurrentState = mmuCacheStatePause

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdPause,
		msg.Src, msg.ID, true, ""))
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

	state := &m.comp.State
	state.InflightFlushReqActive = true
	state.InflightFlushReqID = msg.ID
	state.InflightFlushReqSrc = msg.Src
	m.controlPort().RetrieveIncoming()
	state.CurrentState = mmuCacheStateFlush

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidInCurrentStage(msg messaging.Msg) {
	state := &m.comp.State
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

func (m *ctrlMiddleware) handleReset(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdReset,
		msg.Src, msg.ID, true, ""))
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	state := &m.comp.State
	state.CurrentState = mmuCacheStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.InflightFlushReqActive = false
	state.InflightFlushReqID = 0
	state.InflightFlushReqSrc = ""

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, control.ErrUnsupported))
	m.controlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd mem.ControlCommand,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) *mem.ControlRsp {
	rsp := &mem.ControlRsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "mem.ControlRsp"
	return rsp
}
