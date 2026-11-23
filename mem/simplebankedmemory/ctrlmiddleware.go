package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.Ports.Control
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.Ports.Top
}

func (m *ctrlMiddleware) Handle(e timing.Event) bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != controlStateDraining {
		madeProgress = m.handleIncoming(m.comp.Spec.Freq.Cycle(e.Time())) || madeProgress
	}
	return madeProgress
}

// completePendingDrain finalizes a Drain when no bank has work in
// flight. A bank is quiescent when its pipeline is empty.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != controlStateDraining {
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

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = controlStatePaused
	return true
}

func bankIsQuiescent(b *bankState) bool {
	return b.Pipeline.Size() == 0
}

// freezeBanks stops pipeline time at the end of the cycle before cycle: the
// tick that handles a Pause does no pipeline work.
func (m *ctrlMiddleware) freezeBanks(cycle uint64) {
	for i := range m.comp.State.Banks {
		if p := &m.comp.State.Banks[i].Pipeline; cycle > p.Cycle() {
			p.AdvanceTo(cycle - 1)
		}
	}
}

// resumeBanks discards the paused cycles, so the tick that handles an Enable is
// the first cycle of pipeline work after the freeze.
func (m *ctrlMiddleware) resumeBanks(cycle uint64) {
	for i := range m.comp.State.Banks {
		if p := &m.comp.State.Banks[i].Pipeline; cycle > p.Cycle() {
			p.SkipTo(cycle - 1)
		}
	}
}

func (m *ctrlMiddleware) handleIncoming(cycle uint64) bool {
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
		return m.handlePause(req, cycle)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req, cycle)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req, cycle)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

func (m *ctrlMiddleware) handlePause(req messaging.Msg, cycle uint64) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	if m.comp.State.ControlState != controlStatePaused {
		m.freezeBanks(cycle)
	}
	m.comp.State.ControlState = controlStatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req messaging.Msg, cycle uint64) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	if m.comp.State.ControlState == controlStatePaused {
		m.resumeBanks(cycle)
	}
	m.comp.State.ControlState = controlStateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req messaging.Msg, cycle uint64) bool {
	state := &m.comp.State
	if state.ControlState == controlStatePaused {
		m.resumeBanks(cycle)
	}
	state.ControlState = controlStateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset rebuilds the bank pipelines back to freshly-built shape
// and clears the Top port queue.
func (m *ctrlMiddleware) handleReset(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	m.endInflightTasks()
	state.Banks = buildInitialBanks(m.comp.Name(), m.comp.Spec)
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = controlStateEnabled

	for {
		if _, ok := m.topPort().RetrieveIncoming(); !ok {
			break
		}
	}

	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// endInflightTasks completes the req_in tracing task and closes the open
// pipeline subtask of every request still in a bank pipeline, so a hard Reset
// that rebuilds the banks leaves no started-never-ended task. The memory is
// a leaf, so there is no downstream req_out. Mirrors the completion in
// tickFinalizeMW.finishPipeline + EndReqIn.
func (m *ctrlMiddleware) endInflightTasks() {
	for i := range m.comp.State.Banks {
		bank := &m.comp.State.Banks[i]
		for _, stage := range bank.Pipeline.Stages() {
			m.endItemTasks(stage.Item)
		}
	}
}

func (m *ctrlMiddleware) endItemTasks(item bankPipelineItemState) {
	m.comp.TaskTracker().EndReqIn(item.RecvTaskID)

	if item.PipelineTaskID != 0 {
		m.comp.TaskTracker().EndTask(item.PipelineTaskID)
	}
}

func (m *ctrlMiddleware) handleUnsupported(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.comp, req.Payload.(memcontrolprotocol.Req).Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
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
		ID:    c.NewID(),
		Src:   c.Ports.Control.AsRemote(),
		Dst:   dst,
		RspTo: rspTo,
	}

	return rsp
}
