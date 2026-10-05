package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *Comp
}

// Handle completes a pending Drain and handles the next control command.
func (m *ctrlMiddleware) Handle(_ timing.Event) (madeProgress bool) {
	madeProgress = m.handleStateUpdate() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != memcontrolprotocol.StateDraining {
		madeProgress = m.handleIncomingCommands() || madeProgress
	}
	return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.Ports.Control
}

// handleStateUpdate notices Drain completion and acks the pending
// Drain request, transitioning the component to Paused.
func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
		return false
	}

	if len(state.InflightTransactions) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	rsp := makeRsp(m.comp, memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, "")
	m.ctrlPort().Send(rsp)
	state.ControlState = memcontrolprotocol.StatePaused

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msg, ok := m.ctrlPort().PeekIncoming()
	if !ok {
		return false
	}
	_, ok = msg.Payload.(memcontrolprotocol.Req)
	req := msg
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Payload.(memcontrolprotocol.Req).Command {
	case memcontrolprotocol.CmdPause:
		return m.handlePause(req)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StatePaused

	m.ctrlPort().Send(makeRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateEnabled

	m.ctrlPort().Send(makeRsp(m.comp, memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req messaging.Msg) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleReset(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	m.endInflightTasks()
	state.InflightTransactions = nil
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = memcontrolprotocol.StateEnabled

	// Reset is a hard reset to a freshly-built, no-work state. Drop any
	// requests still queued on the Top port; otherwise (the control
	// middleware runs before the memory middleware) takeNewReqs would consume
	// a stale request in the very same tick, right after the reset ack.
	top := m.comp.Ports.Top
	for {
		if _, ok := top.PeekIncoming(); !ok {
			break
		}
		top.RetrieveIncoming()
	}

	m.ctrlPort().Send(makeRsp(m.comp, memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// endInflightTasks completes the req_in tracing task of every in-flight access
// so a hard Reset that drops InflightTransactions leaves no
// started-never-ended task and no leaked receiver-registry entry. The ideal
// memory controller is a leaf, so there is no downstream req_out to finalize.
// Mirrors the completion in memMiddleware.traceReqComplete.
func (m *ctrlMiddleware) endInflightTasks() {
	for _, tx := range m.comp.State.InflightTransactions {
		tracing.EndReqInOnReset(m.comp, tx.ReqID)
	}
}

func (m *ctrlMiddleware) handleUnsupported(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeRsp(m.comp, req.Payload.(memcontrolprotocol.Req).Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeRsp(
	c *Comp,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) messaging.Msg {
	rsp := messaging.Msg{Payload: memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	},
		ID:           c.NewID(),
		Src:          c.Ports.Control.AsRemote(),
		Dst:          dst,
		RspTo:        rspTo,
		TrafficClass: "memcontrolprotocol.Rsp"}

	return rsp
}
