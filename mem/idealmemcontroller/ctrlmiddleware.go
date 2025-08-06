package idealmemcontroller

import "github.com/sarchlab/akita/v4/mem/mem"

type ctrlMiddleware struct {
	*Comp
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	madeProgress = m.handleIncomingCommands() || madeProgress
	madeProgress = m.handleStateUpdate() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	if m.state == "drain" {
		madeProgress = m.handleDrainState() || madeProgress
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleDrainState() bool {
	if len(m.inflightBuffer) != 0 {
		return false
	}

	rsp := m.currentCmd.GenerateRsp()

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.state = "pause"
	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
	msg := m.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	ctrlMsg := msg.(*mem.ControlMsg)

	m.ctrlMsgMustBeValid(ctrlMsg)

	madeProgress = m.handleEnable(ctrlMsg) || madeProgress
	madeProgress = m.handlePause(ctrlMsg) || madeProgress
	madeProgress = m.handleDrain(ctrlMsg) || madeProgress

	return madeProgress
}

func (m *ctrlMiddleware) handleEnable(ctrlMsg *mem.ControlMsg) bool {
	if ctrlMsg.Enable {
		m.state = "enable"
		rsp := ctrlMsg.GenerateRsp()

		err := m.ctrlPort.Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handlePause(ctrlMsg *mem.ControlMsg) bool {
	if !ctrlMsg.Enable && !ctrlMsg.Drain {
		m.state = "pause"
		rsp := ctrlMsg.GenerateRsp()

		err := m.ctrlPort.Send(rsp)
		if err != nil {
			return false
		}

		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) handleDrain(ctrlMsg *mem.ControlMsg) bool {
	if !ctrlMsg.Enable && ctrlMsg.Drain {
		m.state = "drain"
		m.currentCmd = ctrlMsg
		m.ctrlPort.RetrieveIncoming()
		return true
	}

	return false
}

func (m *ctrlMiddleware) ctrlMsgMustBeValid(ctrlMsg *mem.ControlMsg) {
	if ctrlMsg.Enable {
		if ctrlMsg.Drain {
			panic("Enable and Drain should not be set at the same time")
		}

		if ctrlMsg.Invalid {
			panic("Enable and Invalid should not be set at the same time")
		}

		if ctrlMsg.Flush {
			panic("Enable and Flush should not be set at the same time")
		}
	}

	if !ctrlMsg.Enable {
		m.drainSignalMustNotInvalidate(ctrlMsg)
	}
}

func (m *ctrlMiddleware) drainSignalMustNotInvalidate(ctrlMsg *mem.ControlMsg) {
	if ctrlMsg.Drain && ctrlMsg.Invalid {
		panic("Drain and Invalid should not be set at the same time")
	}

	if ctrlMsg.Drain && ctrlMsg.Flush {
		panic("Drain and Flush should not be set at the same time")
	}
}
