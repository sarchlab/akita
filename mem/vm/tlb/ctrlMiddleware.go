package tlb

import (
	"github.com/sarchlab/akita/v4/mem/mem"
)

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
	case *FlushReq:
		madeProgress = m.handleFlushReq(msg) || madeProgress
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
		if m.state != "enable" {
			m.state = "enable"
			m.isPaused = false
		}
	} else if msg.Drain {
		if m.state != "drain" {
			m.state = "drain"
			m.isPaused = false
		}
	} else if msg.Pause {
		if m.state != "pause" {
			m.state = "pause"
			m.isPaused = true
		}
	} else {
		panic("Invalid control message")
	}
}

func (m *ctrlMiddleware) handleFlushReq(msg *FlushReq) (madeProgress bool) {
	rsp := FlushRspBuilder{}.
		WithSrc(m.controlPort.AsRemote()).
		WithDst(msg.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	for _, vAddr := range msg.VAddr {
		setID := m.vAddrToSetID(vAddr)
		set := m.sets[setID]
		wayID, page, found := set.Lookup(msg.PID, vAddr)
		if !found {
			continue
		}

		page.Valid = false
		set.Update(wayID, page)
	}

	m.mshr.Reset()
	// m.isPaused = true
	m.state = "pause"
	return true
}

func (m *ctrlMiddleware) vAddrToSetID(vAddr uint64) (setID int) {
	return int(vAddr / m.pageSize % uint64(m.numSets))
}
