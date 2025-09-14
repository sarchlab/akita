package writearound

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

type ctrlMiddleware struct {
	*Comp
	inflightCtrlMsg *mem.ControlMsg

	ctrlPort sim.Port
	// transactions *[]*transaction
	directory cache.Directory
	cache     *Comp
	// coalescer    *coalescer
	// bankStages   []*bankStage

	// currFlushReq *cache.FlushReq
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
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

func (m *ctrlMiddleware) handleControlMsg(msg *mem.ControlMsg) (madeProgress bool) {
	madeProgress = false
	m.ctrlMsgMustBeValidinCurrentState(msg)
	madeProgress = m.handleEnable(msg) || madeProgress
	madeProgress = m.handlePause(msg) || madeProgress
	madeProgress = m.handleDrainStage(msg) || madeProgress
	madeProgress = m.handleFlush(msg) || madeProgress
	// madeProgress = m.handleInvalid(msg) || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidinCurrentState(msg *mem.ControlMsg) {
	switch m.state {
	case "enable":
		if msg.Enable {
			panic("Enable has already been set")
		}
		break
	case "pause":
		if msg.Pause {
			panic("Pause has already been set")
		}

		if msg.Drain || msg.Flush || msg.Invalid {
			panic("Drain, Flush, and Invalid cannot be set in the pause state")
		}
	case "flush":
		if msg.Enable || msg.Drain || msg.Invalid {
			panic("Enable, Drain, and Invalid cannot be set in the flush state")
		}
	case "drain":
		if msg.Enable || msg.Flush || msg.Invalid {
			panic("Enable, Flush, and Invalid cannot be set in the drain state")
		}
	case "drainDone":
		if msg.Enable || msg.Flush || msg.Invalid {
			panic("Enable, Flush, and Invalid cannot be set in the drainDone state")
		}
	case "invalid":
		if msg.Enable || msg.Drain || msg.Flush {
			panic("Enable, Drain, and Flush cannot be set in the invalid state")
		}
	default:
		break
	}
}

func (m *ctrlMiddleware) handleEnable(msg *mem.ControlMsg) (madeProgress bool) {
	if msg.Enable {
		m.state = "enable"
		rsp := mem.ControlMsgRspBuilder{}.
			WithSrc(msg.Dst).
			WithDst(msg.Src).
			WithRspTo(msg.ID).
			WithEnable(true).
			WithDrain(msg.Drain).
			WithFlush(msg.Flush).
			WithPause(msg.Pause).
			WithInvalid(msg.Invalid).
			Build()

		err := m.controlPort.Send(rsp)
		if err != nil {
			return false
		}
		m.controlPort.RetrieveIncoming()
		return true
	}
	return false
}

func (m *ctrlMiddleware) handlePause(msg *mem.ControlMsg) (madeProgress bool) {
	if msg.Pause {
		m.state = "pause"
		rsp := mem.ControlMsgRspBuilder{}.
			WithSrc(msg.Dst).
			WithDst(msg.Src).
			WithRspTo(msg.ID).
			WithEnable(msg.Enable).
			WithDrain(msg.Drain).
			WithFlush(msg.Flush).
			WithPause(true).
			WithInvalid(msg.Invalid).
			Build()
		err := m.controlPort.Send(rsp)
		if err != nil {
			return false
		}
		m.controlPort.RetrieveIncoming()
		return true
	}
	return false
}

func (m *ctrlMiddleware) handleDrainStage(msg *mem.ControlMsg) (madeProgress bool) {
	madeProgress = m.handleDrain(msg) || madeProgress
	madeProgress = m.handleDrainDone() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleDrain(msg *mem.ControlMsg) (madeProgress bool) {
	if msg.Drain {
		m.state = "drain"
		m.inflightCtrlMsg = msg
		return true
	}
	return false
}

func (m *ctrlMiddleware) handleDrainDone() (madeProgress bool) {
	if m.state != "drainDone" {
		return false
	}

	if m.inflightCtrlMsg.Drain {
		m.state = "pause"

		rsp := mem.ControlMsgRspBuilder{}.
			WithSrc(m.inflightCtrlMsg.Dst).
			WithDst(m.inflightCtrlMsg.Src).
			WithRspTo(m.inflightCtrlMsg.ID).
			WithEnable(m.inflightCtrlMsg.Enable).
			WithDrain(m.inflightCtrlMsg.Drain).
			WithFlush(m.inflightCtrlMsg.Flush).
			WithPause(m.inflightCtrlMsg.Pause).
			WithInvalid(m.inflightCtrlMsg.Invalid).
			Build()

		err := m.controlPort.Send(rsp)
		if err != nil {
			return false
		}
		m.controlPort.RetrieveIncoming()
		m.inflightCtrlMsg = nil
		return true
	} else if m.inflightCtrlMsg.Flush {
		m.state = "flush"
		return true
	}
	return false
}

func (m *ctrlMiddleware) handleFlush(msg *mem.ControlMsg) (madeProgress bool) {
	madeProgress = m.handleFlushReq(msg) || madeProgress
	madeProgress = m.handleFlushStage(msg) || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleFlushReq(msg *mem.ControlMsg) (madeProgress bool) {
	if msg.Flush {
		m.state = "drain"
		m.inflightCtrlMsg = msg
		return true
	}
	return false
}

func (m *ctrlMiddleware) handleFlushStage(msg *mem.ControlMsg) (madeProgress bool) {
	if m.inflightCtrlMsg == nil {
		return false
	}

	if !m.inflightCtrlMsg.Flush {
		return false
	}

	if m.state != "flush" {
		return false
	}

	rsp := cache.FlushRspBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(m.inflightCtrlMsg.Src).
		WithRspTo(m.inflightCtrlMsg.ID).
		Build()

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.hardResetCache()
	m.inflightCtrlMsg = nil

	return true
}

func (m *ctrlMiddleware) hardResetCache() {
	m.flushPort(m.cache.topPort)
	m.flushPort(m.cache.bottomPort)
	m.flushBuffer(m.cache.dirBuf)

	for _, bankBuf := range m.cache.bankBufs {
		m.flushBuffer(bankBuf)
	}

	m.directory.Reset()
	m.cache.mshr.Reset()
	m.cache.coalesceStage.Reset()

	for _, bankStage := range m.cache.bankStages {
		bankStage.Reset()
	}

	m.cache.transactions = nil
	m.cache.postCoalesceTransactions = nil

	m.state = "pause"
}

func (m *ctrlMiddleware) flushPort(port sim.Port) {
	for port.PeekIncoming() != nil {
		port.RetrieveIncoming()
	}
}

func (m *ctrlMiddleware) flushBuffer(buffer sim.Buffer) {
	for buffer.Pop() != nil {
	}
}
