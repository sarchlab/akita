package simplebankedmemory

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

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	// Reset is the highest-priority verb: when one is queued it preempts a
	// pending async verb, so skip completing the drain this tick; any other
	// verb lets the pending drain finish first.
	if !control.IsResetPending(m.ctrlPort()) {
		madeProgress = m.completePendingDrain() || madeProgress
	}
	madeProgress = m.handleIncoming() || madeProgress
	return madeProgress
}

// completePendingDrain finalizes a Drain when no bank has work in
// flight. A bank is quiescent when its pipeline reports empty and its
// post-pipeline buffer is empty.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != control.StateDraining {
		return false
	}

	for i := range state.Banks {
		if !bankIsQuiescent(&state.Banks[i]) {
			return false
		}
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = control.StatePaused
	return true
}

func bankIsQuiescent(b *bankState) bool {
	return len(b.Pipeline.Stages()) == 0 && b.PostPipelineBuf.Size() == 0
}

func (m *ctrlMiddleware) handleIncoming() bool {
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
	// Only Reset preempts an in-flight async verb: a Pause must not abort a
	// Drain in progress. Leave the draining state so the drain finishes and
	// lands in paused on its own.
	if m.comp.State.ControlState != control.StateDraining {
		m.comp.State.ControlState = control.StatePaused
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = control.StateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdEnable,
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

// handleReset rebuilds the bank pipelines back to freshly-built shape
// and clears the Top port queue.
func (m *ctrlMiddleware) handleReset(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.Banks = buildInitialBanks(m.comp.Spec())
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = control.StateEnabled

	for m.topPort().RetrieveIncoming() != nil {
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, control.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
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
