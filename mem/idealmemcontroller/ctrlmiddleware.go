package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	// Reset is the highest-priority verb: when one is queued it preempts a
	// pending async verb (handled in handleIncomingCommands), so skip
	// completing the drain this tick; any other verb lets the pending drain
	// finish first.
	if !control.IsResetPending(m.ctrlPort()) {
		madeProgress = m.handleStateUpdate() || madeProgress
	}
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

// handleStateUpdate notices Drain completion and acks the pending
// Drain request, transitioning the component to Paused.
func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := &m.comp.State
	if state.ControlState != control.StateDraining {
		return false
	}

	if len(state.InflightTransactions) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	rsp := makeRsp(m.ctrlPort(), mem.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, "")
	m.ctrlPort().Send(rsp)
	state.ControlState = control.StatePaused

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(mem.ControlReq)
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
	case mem.CmdPause:
		return m.handlePause(req)
	case mem.CmdDrain:
		return m.handleDrain(req)
	case mem.CmdEnable:
		return m.handleEnable(req)
	case mem.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	// Only Reset preempts an in-flight async verb: a Pause must not abort a
	// Drain in progress. Leave the draining state so the drain finishes and
	// lands in paused on its own.
	if state.ControlState != control.StateDraining {
		state.ControlState = control.StatePaused
	}

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), mem.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.ControlState = control.StateEnabled

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), mem.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req mem.ControlReq) bool {
	state := &m.comp.State
	state.ControlState = control.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleReset(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.InflightTransactions = nil
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = control.StateEnabled

	// Reset is a hard reset to a freshly-built, no-work state. Drop any
	// requests still queued on the Top port; otherwise (the control
	// middleware runs before the memory middleware) takeNewReqs would consume
	// a stale request in the very same tick, right after the reset ack.
	top := m.comp.GetPortByName("Top")
	for top.PeekIncoming() != nil {
		top.RetrieveIncoming()
	}

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), mem.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, control.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeRsp(
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
