package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	madeProgress = m.handleIncomingCommands() || madeProgress
	madeProgress = m.handleStateUpdate() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := &m.comp.State
	if state.CurrentState == "drain" {
		madeProgress = m.handleDrainState() || madeProgress
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleDrainState() bool {
	state := &m.comp.State
	if len(state.InflightTransactions) != 0 {
		return false
	}

	rsp := &mem.ControlRsp{Command: mem.CmdDrain, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = state.CurrentCmdSrc
	rsp.RspTo = state.CurrentCmdID
	rsp.TrafficClass = "mem.ControlRsp"

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(rsp)

	state.CurrentState = "pause"

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msgI := m.ctrlPort().PeekIncoming()
	if msgI == nil {
		return false
	}

	msg := msgI.(*mem.ControlReq)

	switch msg.Command {
	case mem.CmdEnable:
		madeProgress = m.handleEnable(msg) || madeProgress
	case mem.CmdPause:
		madeProgress = m.handlePause(msg) || madeProgress
	case mem.CmdDrain:
		madeProgress = m.handleDrain(msg) || madeProgress
	default:
		// Immediate ack for unhandled commands
		m.ctrlPort().RetrieveIncoming()
		madeProgress = true
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleEnable(
	msg *mem.ControlReq,
) bool {
	state := &m.comp.State
	state.CurrentState = "enable"

	rsp := &mem.ControlRsp{Command: mem.CmdEnable, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "mem.ControlRsp"

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(rsp)

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handlePause(
	msg *mem.ControlReq,
) bool {
	state := &m.comp.State
	state.CurrentState = "pause"

	rsp := &mem.ControlRsp{Command: mem.CmdPause, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "mem.ControlRsp"

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(rsp)

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(
	msg *mem.ControlReq,
) bool {
	state := &m.comp.State
	state.CurrentState = "drain"
	state.CurrentCmdID = msg.ID
	state.CurrentCmdSrc = msg.Src

	m.ctrlPort().RetrieveIncoming()
	return true
}
