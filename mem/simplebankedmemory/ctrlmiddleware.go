package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
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

func (m *ctrlMiddleware) Handle(_ timing.Event) bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != memcontrolprotocol.StateDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

// completePendingDrain finalizes a Drain when no bank has work in
// flight. A bank is quiescent when its pipeline reports empty and its
// post-pipeline buffer is empty.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
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
	state.ControlState = memcontrolprotocol.StatePaused
	return true
}

func bankIsQuiescent(b *bankState) bool {
	return len(b.Pipeline.Stages()) == 0 && b.PostPipelineBuf.Size() == 0
}

func (m *ctrlMiddleware) handleIncoming() bool {
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
	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req messaging.Msg) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.comp, memcontrolprotocol.CmdEnable,
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
	state.ControlState = memcontrolprotocol.StateEnabled

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
// pipeline subtask of every request still in a bank pipeline or post-pipeline
// buffer, so a hard Reset that rebuilds the banks leaves no
// started-never-ended task and no leaked receiver-registry entry. The memory is
// a leaf, so there is no downstream req_out. Mirrors the completion in
// tickFinalizeMW.finishPipeline + TraceReqComplete.
func (m *ctrlMiddleware) endInflightTasks() {
	for i := range m.comp.State.Banks {
		bank := &m.comp.State.Banks[i]
		for _, stage := range bank.Pipeline.Stages() {
			m.endItemTasks(stage.Item)
		}
		for _, item := range bank.PostPipelineBuf.Elements() {
			m.endItemTasks(item)
		}
	}
}

func (m *ctrlMiddleware) endItemTasks(item bankPipelineItemState) {
	reqMsgID := item.WriteMsg.ID
	if item.IsRead {
		reqMsgID = item.ReadMsg.ID
	}

	tracing.EndReqInOnReset(m.comp, reqMsgID)

	if item.PipelineTaskID != 0 {
		tracing.EndTaskOnReset(m.comp, item.PipelineTaskID)
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
		ID:           c.NewID(),
		Src:          c.Ports.Control.AsRemote(),
		Dst:          dst,
		RspTo:        rspTo,
		TrafficClass: "memcontrolprotocol.Rsp"}

	return rsp
}
