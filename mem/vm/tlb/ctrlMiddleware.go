package tlb

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/tracing"
)

type ctrlMiddleware struct {
	*Comp
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
	case *FlushReq:
		madeProgress = m.handleTLBFlush(msg) || madeProgress
	case *RestartReq:
		madeProgress = m.handleTLBRestart(msg) || madeProgress
	default:
		panic("Unhandled message")
	}

	return madeProgress
}

func (m *ctrlMiddleware) handleControlMsg(
	msg *mem.ControlMsg) bool {
	m.ctrlMsgMustBeValidinCurrentStage(msg)

	return m.performCtrlReq(msg)
}

func (m *ctrlMiddleware) ctrlMsgMustBeValidinCurrentStage(msg *mem.ControlMsg) {
	switch state := m.state; state {
	case "enable":
		if msg.Enable {
			log.Panic("TLB is already enabled at %s", m.Name())
		}
	case "pause":
		if msg.Pause {
			log.Panic("TLB is already paused at %s", m.Name())
		}
		if msg.Drain {
			log.Panic("Cannot drain when TLB is paused at %s", m.Name())
		}
	case "drain":
		if msg.Drain {
			log.Panic("TLB is already draining at %s", m.Name())
		}
		if msg.Pause || msg.Enable {
			log.Panic("Cannot pause/enable when TLB is draining at %s", m.Name())
		}
	case "flush":
		if msg.Drain || msg.Enable || msg.Pause {
			log.Panic("Cannot pause/enable/drain when TLB is flushing at %s", m.Name())
		}
	default:
		log.Panic("Unknown TLB state: %s", state)
	}
}

func (m *ctrlMiddleware) performCtrlReq(req *mem.ControlMsg) bool {
	item := m.controlPort.PeekIncoming()
	if item == nil {
		return false
	}

	item = m.controlPort.RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(item, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	if req.Enable {
		m.state = "enable"
	} else if req.Drain {
		m.state = "drain"
	} else if req.Pause {
		m.state = "pause"
	}

	return true
}

func (m *ctrlMiddleware) handleTLBFlush(req *FlushReq) bool {
	m.flushMsgMustBeValidinCurrentStage(req)
	m.inflightFlushReq = req
	m.state = "flush"

	return true
}

func (m *ctrlMiddleware) flushMsgMustBeValidinCurrentStage(req *FlushReq) {
	switch state := m.state; state {
	case "enable":
		// valid
	case "pause":
		log.Panic("Cannot flush when TLB is paused at %s", m.Name())
	case "drain":
		log.Panic("Cannot flush when TLB is draining at %s", m.Name())
	case "flush":
		log.Panic("TLB is already flushing at %s", m.Name())
	default:
		log.Panic("Unknown TLB state: %s", state)
	}
}

func (m *ctrlMiddleware) handleTLBRestart(req *RestartReq) bool {
	rsp := RestartRspBuilder{}.
		WithSrc(m.controlPort.AsRemote()).
		WithDst(req.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)
	m.isPaused = false

	for m.topPort.RetrieveIncoming() != nil {
		m.topPort.RetrieveIncoming()
	}

	for m.bottomPort.RetrieveIncoming() != nil {
		m.bottomPort.RetrieveIncoming()
	}

	return true
}

func (m *ctrlMiddleware) vAddrToSetID(vAddr uint64) (setID int) {
	return int(vAddr / m.pageSize % uint64(m.numSets))
}
