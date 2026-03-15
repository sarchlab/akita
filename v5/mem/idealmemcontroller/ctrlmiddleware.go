package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	madeProgress = m.handleIncomingCommands() || madeProgress
	madeProgress = m.handleStateUpdate() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := m.comp.GetNextState()
	if state.CurrentState == "drain" {
		madeProgress = m.handleDrainState() || madeProgress
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleDrainState() bool {
	state := m.comp.GetNextState()
	if len(state.InflightTransactions) != 0 {
		return false
	}

	rsp := &mem.ControlMsgRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = state.CurrentCmdSrc
	rsp.RspTo = state.CurrentCmdID
	rsp.TrafficClass = "mem.ControlMsgRsp"

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	state.CurrentState = "pause"

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msgI := m.ctrlPort().PeekIncoming()
	if msgI == nil {
		return false
	}

	msg := msgI.(*mem.ControlMsg)

	m.ctrlMsgMustBeValid(msg)

	madeProgress = m.handleEnable(msg) || madeProgress
	madeProgress = m.handlePause(msg) || madeProgress
	madeProgress = m.handleDrain(msg) || madeProgress

	return madeProgress
}

func (m *ctrlMiddleware) handleEnable(
	msg *mem.ControlMsg,
) bool {
	if msg.Enable {
		state := m.comp.GetNextState()
		state.CurrentState = "enable"

		rsp := &mem.ControlMsgRsp{}
		rsp.ID = sim.GetIDGenerator().Generate()
		rsp.Src = m.ctrlPort().AsRemote()
		rsp.Dst = msg.Src
		rsp.RspTo = msg.ID
		rsp.Enable = true
		rsp.TrafficClass = "mem.ControlMsgRsp"

		err := m.ctrlPort().Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort().RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handlePause(
	msg *mem.ControlMsg,
) bool {
	if !msg.Enable && !msg.Drain {
		state := m.comp.GetNextState()
		state.CurrentState = "pause"

		rsp := &mem.ControlMsgRsp{}
		rsp.ID = sim.GetIDGenerator().Generate()
		rsp.Src = m.ctrlPort().AsRemote()
		rsp.Dst = msg.Src
		rsp.RspTo = msg.ID
		rsp.Pause = true
		rsp.TrafficClass = "mem.ControlMsgRsp"

		err := m.ctrlPort().Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort().RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handleDrain(
	msg *mem.ControlMsg,
) bool {
	if !msg.Enable && msg.Drain {
		state := m.comp.GetNextState()
		state.CurrentState = "drain"
		state.CurrentCmdID = msg.ID
		state.CurrentCmdSrc = msg.Src

		m.ctrlPort().RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) ctrlMsgMustBeValid(msg *mem.ControlMsg) {
	if msg.Enable {
		if msg.Drain {
			panic("Enable and Drain should not be set at the same time")
		}

		if msg.Invalid {
			panic("Enable and Invalid should not be set at the same time")
		}

		if msg.Flush {
			panic("Enable and Flush should not be set at the same time")
		}
	}

	if !msg.Enable {
		m.drainSignalMustNotInvalidate(msg)
	}
}

func (m *ctrlMiddleware) drainSignalMustNotInvalidate(msg *mem.ControlMsg) {
	if msg.Drain && msg.Invalid {
		panic("Drain and Invalid should not be set at the same time")
	}

	if msg.Drain && msg.Flush {
		panic("Drain and Flush should not be set at the same time")
	}
}
