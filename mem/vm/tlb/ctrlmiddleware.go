package tlb

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) controlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

// completePendingDrain detects that the data-path middleware has
// finished draining (TLBState transitioned to pause) and sends the
// async Drain Rsp.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if !state.PendingDrainRsp {
		return false
	}
	if state.TLBState != tlbStatePause {
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
	msg := m.controlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	ctrlReq, ok := msg.(*mem.ControlReq)
	if !ok {
		return false
	}

	switch ctrlReq.Command {
	case mem.CmdEnable:
		return m.performCtrlEnable(ctrlReq)
	case mem.CmdDrain:
		return m.performCtrlDrain(ctrlReq)
	case mem.CmdPause:
		return m.performCtrlPause(ctrlReq)
	case mem.CmdFlush:
		// TODO(control-protocol): Phase 3 splits this verb. The current
		// implementation treats CmdFlush as an Invalidate-with-filter,
		// which is what TLBs actually need. Phase 3 renames the
		// handler to CmdInvalidate and marks CmdFlush unsupported.
		return m.handleTLBFlush(ctrlReq)
	case mem.CmdReset:
		return m.handleReset(ctrlReq)
	case mem.CmdInvalidate:
		return m.handleUnsupported(ctrlReq)
	default:
		return m.handleUnsupported(ctrlReq)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg *mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.TLBState = tlbStateEnable

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
	state.TLBState = tlbStateDrain
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
	state.TLBState = tlbStatePause

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

func (m *ctrlMiddleware) handleTLBFlush(msg *mem.ControlReq) bool {
	state := &m.comp.State
	state.HasInflightFlushReq = true
	state.InflightFlush = inflightFlushState{
		VAddr: msg.Addresses,
		PID:   msg.PID,
		Meta:  msg.MsgMeta,
	}
	m.controlPort().RetrieveIncoming()
	state.TLBState = tlbStateFlush
	return true
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
	state.TLBState = tlbStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	// Reset is a hard reset: in-flight misses are discarded, not preserved.
	state.MSHREntries = nil
	state.HasRespondingMSHR = false
	state.RespondingMSHRData = mshrEntryState{}

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
