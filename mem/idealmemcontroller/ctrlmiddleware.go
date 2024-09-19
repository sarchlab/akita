package idealmemcontroller

import "github.com/sarchlab/akita/v4/mem/mem"

type ctrlMiddleware struct {
	*Comp
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	msg := m.ctrlPort.RetrieveIncoming()
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
		m.ctrlPort.Send(rsp)
		return true
	}
	return false
}

func (m *ctrlMiddleware) handlePause(ctrlMsg *mem.ControlMsg) bool {
	if !ctrlMsg.Enable && !ctrlMsg.Drain {
		m.state = "pause"
		rsp := ctrlMsg.GenerateRsp()
		m.ctrlPort.Send(rsp)
		return true
	}

	return false
}

func (m *ctrlMiddleware) handleDrain(ctrlMsg *mem.ControlMsg) bool {
	if !ctrlMsg.Enable && ctrlMsg.Drain {
		m.state = "drain"
		m.respondReq = ctrlMsg
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
		if ctrlMsg.Drain {
			if ctrlMsg.Invalid {
				panic("Drain and Invalid should not be set at the same time")
			}

			if ctrlMsg.Flush {
				panic("Drain and Flush should not be set at the same time")
			}
		}
	}
}
