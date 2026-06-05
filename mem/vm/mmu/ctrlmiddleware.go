package mmu

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
	madeProgress = m.completePendingDrain() || madeProgress
	madeProgress = m.handleIncoming() || madeProgress
	return madeProgress
}

// completePendingDrain notices Drain has reached quiescence and acks.
// Quiescence means no page-table walks are in progress.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != control.StateDraining {
		return false
	}

	if len(state.WalkingTranslations) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), mem.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = control.StatePaused

	return true
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
	m.comp.State.ControlState = control.StatePaused
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

// handleReset clears all local MMU bookkeeping back to a freshly-built
// state. Shared page-table state lives in the simulation and is not
// touched here; per the checkpointing model the MMU only owns local
// walk state.
func (m *ctrlMiddleware) handleReset(req mem.ControlReq) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.WalkingTranslations = nil
	state.ToRemoveFromPTW = nil
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
