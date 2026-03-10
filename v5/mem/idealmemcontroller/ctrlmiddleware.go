package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/sim"
)

type ctrlMiddleware struct {
	*Comp
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	madeProgress = m.handleIncomingCommands() || madeProgress
	madeProgress = m.handleStateUpdate() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := m.Component.GetState()
	if state.CurrentState == "drain" {
		madeProgress = m.handleDrainState() || madeProgress
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleDrainState() bool {
	state := m.Component.GetState()
	if len(state.InflightTransactions) != 0 {
		return false
	}

	rsp := mem.ControlMsgRspBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(state.CurrentCmdSrc).
		WithRspTo(state.CurrentCmdID).
		Build()

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	state.CurrentState = "pause"
	m.Component.SetState(state)

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msg := m.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	ctrlPayload := sim.MsgPayload[mem.ControlMsgPayload](msg)

	m.ctrlMsgMustBeValid(ctrlPayload)

	madeProgress = m.handleEnable(msg, ctrlPayload) || madeProgress
	madeProgress = m.handlePause(msg, ctrlPayload) || madeProgress
	madeProgress = m.handleDrain(msg, ctrlPayload) || madeProgress

	return madeProgress
}

func (m *ctrlMiddleware) handleEnable(
	msg *sim.Msg,
	ctrlPayload *mem.ControlMsgPayload,
) bool {
	if ctrlPayload.Enable {
		state := m.Component.GetState()
		state.CurrentState = "enable"
		m.Component.SetState(state)

		rsp := mem.ControlMsgRspBuilder{}.
			WithSrc(m.ctrlPort.AsRemote()).
			WithDst(msg.Src).
			WithRspTo(msg.ID).
			WithEnable(true).
			Build()

		err := m.ctrlPort.Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handlePause(
	msg *sim.Msg,
	ctrlPayload *mem.ControlMsgPayload,
) bool {
	if !ctrlPayload.Enable && !ctrlPayload.Drain {
		state := m.Component.GetState()
		state.CurrentState = "pause"
		m.Component.SetState(state)

		rsp := mem.ControlMsgRspBuilder{}.
			WithSrc(m.ctrlPort.AsRemote()).
			WithDst(msg.Src).
			WithRspTo(msg.ID).
			WithPause(true).
			Build()

		err := m.ctrlPort.Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handleDrain(
	msg *sim.Msg,
	ctrlPayload *mem.ControlMsgPayload,
) bool {
	if !ctrlPayload.Enable && ctrlPayload.Drain {
		state := m.Component.GetState()
		state.CurrentState = "drain"
		state.CurrentCmdID = msg.ID
		state.CurrentCmdSrc = msg.Src
		m.Component.SetState(state)

		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) ctrlMsgMustBeValid(ctrlPayload *mem.ControlMsgPayload) {
	if ctrlPayload.Enable {
		if ctrlPayload.Drain {
			panic("Enable and Drain should not be set at the same time")
		}

		if ctrlPayload.Invalid {
			panic("Enable and Invalid should not be set at the same time")
		}

		if ctrlPayload.Flush {
			panic("Enable and Flush should not be set at the same time")
		}
	}

	if !ctrlPayload.Enable {
		m.drainSignalMustNotInvalidate(ctrlPayload)
	}
}

func (m *ctrlMiddleware) drainSignalMustNotInvalidate(ctrlPayload *mem.ControlMsgPayload) {
	if ctrlPayload.Drain && ctrlPayload.Invalid {
		panic("Drain and Invalid should not be set at the same time")
	}

	if ctrlPayload.Drain && ctrlPayload.Flush {
		panic("Drain and Flush should not be set at the same time")
	}
}
