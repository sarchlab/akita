package tlb

import "github.com/sarchlab/akita/v4/mem/mem"

type ctrlMiddleware struct {
	*Comp
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
	// madeProgress = m.handleStatusUpdate() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	madeProgress := false
	msg := m.controlPort.PeekIncoming()

	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *mem.ControlMsg:
		madeProgress = m.handleControlMsg(msg) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(
	msg *mem.ControlMsg) (madeProgress bool) {
	m.ctrlMsgMustBeValid(msg)
	return madeProgress
}

func (m *ctrlMiddleware) ctrlMsgMustBeValid(msg *mem.ControlMsg) {
	if msg.Enable {

	}
}

// func (m *ctrlMiddleware) handleStatusUpdate() bool {

// }
