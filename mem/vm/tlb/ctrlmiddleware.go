package tlb

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
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
	// Reset is the highest-priority verb: service incoming control (a Reset
	// preempts the in-progress async verb) before completing a pending drain.
	madeProgress = m.handleIncomingCommands() || madeProgress
	madeProgress = m.completePendingDrain() || madeProgress
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

	ctrlReq, ok := msg.(mem.ControlReq)
	if !ok {
		// Drop unexpected message types so the Control port does not stall.
		m.controlPort().RetrieveIncoming()
		return true
	}

	switch ctrlReq.Command {
	case mem.CmdEnable:
		return m.performCtrlEnable(ctrlReq)
	case mem.CmdDrain:
		return m.performCtrlDrain(ctrlReq)
	case mem.CmdPause:
		return m.performCtrlPause(ctrlReq)
	case mem.CmdReset:
		return m.handleReset(ctrlReq)
	case mem.CmdInvalidate:
		return m.handleInvalidate(ctrlReq)
	case mem.CmdFlush:
		// A TLB holds no dirty data, so Flush is not meaningful; callers
		// drop entries with Invalidate instead.
		return m.handleUnsupported(ctrlReq)
	default:
		return m.handleUnsupported(ctrlReq)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg mem.ControlReq) bool {
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
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlDrain(msg mem.ControlReq) bool {
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
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlPause(msg mem.ControlReq) bool {
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
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// handleInvalidate drops cached translations matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). Invalidate is a synchronous verb but is only legal once the TLB
// is paused or drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(msg mem.ControlReq) bool {
	state := &m.comp.State
	// Invalidate is only legal once the TLB is fully paused. While it is
	// still draining, in-flight bottom responses can still be parsed into
	// the sets after the invalidate, so accept only the paused state.
	if state.TLBState != tlbStatePause {
		return m.rejectMustBePaused(msg)
	}
	if !m.controlPort().CanSend() {
		return false
	}

	invalidateEntries(state, m.comp.Spec(), msg.Addresses, msg.PID)

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), mem.CmdInvalidate,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindDependency,
		m.comp.Name()+".Sets",
		m.comp.Name(),
		m.comp,
	)
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while the
// component is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(msg mem.ControlReq) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, control.ErrMustBePausedOrDrained))
	m.controlPort().RetrieveIncoming()
	return true
}

// invalidateEntries marks every cached page matching the filter invalid.
func invalidateEntries(
	state *State,
	spec Spec,
	addresses []uint64,
	pid vm.PID,
) {
	matchAddr := make(map[uint64]bool, len(addresses))
	for _, a := range addresses {
		matchAddr[a/spec.PageSize*spec.PageSize] = true
	}

	for si := range state.Sets {
		set := &state.Sets[si]
		for wi := range set.Blocks {
			page := set.Blocks[wi].Page
			if !page.Valid {
				continue
			}
			if pid != 0 && page.PID != pid {
				continue
			}
			if len(addresses) > 0 && !matchAddr[page.VAddr] {
				continue
			}
			page.Valid = false
			setUpdate(set, wi, page)
		}
	}
}

func (m *ctrlMiddleware) handleReset(msg mem.ControlReq) bool {
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
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	state := &m.comp.State
	state.TLBState = tlbStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	// Reset is a hard reset to the freshly-built shape: discard in-flight
	// misses, the cached translations, and any staged pipeline work.
	state.MSHREntries = nil
	state.HasRespondingMSHR = false
	state.RespondingMSHRData = mshrEntryState{}

	spec := m.comp.Spec()
	state.Sets = initSets(spec.NumSets, spec.NumWays)
	state.Pipeline.Clear()
	state.BufferItems.Clear()

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}
	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(msg mem.ControlReq) bool {
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
) mem.ControlRsp {
	rsp := mem.ControlRsp{
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
