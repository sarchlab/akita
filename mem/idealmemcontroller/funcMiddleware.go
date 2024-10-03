package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type funcMiddleware struct {
	*Comp
}

func (m *funcMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.tickInflightBuffer() || madeProgress
	madeProgress = m.checkAndExecuteState() || madeProgress

	return madeProgress
}

func (m *funcMiddleware) checkAndExecuteState() bool {
	madeProgress := false

	switch state := m.state; state {
	case "enable":
		madeProgress = m.handleInflightMemReqs()
	case "pause":
		madeProgress = true
	case "drain":
		madeProgress = m.handleDrainReq()
	}

	return madeProgress
}

func (m *funcMiddleware) handleDrainReq() bool {
	madeProgress := false
	for len(m.inflightbuffer) != 0 {
		madeProgress = m.handleMemReqs()
	}
	if !m.setState("pause", m.respondReq) {
		return true
	}
	return madeProgress
}

func (m *funcMiddleware) tickInflightBuffer() bool {
	if m.state == "pause" {
		return false
	}

	for i := 0; i < m.width; i++ {
		msg := m.topPort.RetrieveIncoming()
		if msg == nil {
			return false
		}

		m.inflightbuffer = append(m.inflightbuffer, msg)
	}
	return true
}

// updateMemCtrl updates ideal memory controller state.
func (m *funcMiddleware) handleInflightMemReqs() bool {
	madeProgress := false
	for i := 0; i < m.width; i++ {
		madeProgress = m.handleMemReqs()
	}

	return madeProgress
}

func (m *funcMiddleware) handleMemReqs() bool {
	if len(m.inflightbuffer) == 0 {
		return false
	}

	msg := m.inflightbuffer[0]
	m.inflightbuffer = m.inflightbuffer[1:]

	tracing.TraceReqReceive(msg, m)

	switch msg := msg.(type) {
	case *mem.ReadReq:
		m.handleReadReq(msg)
		return true
	case *mem.WriteReq:
		m.handleWriteReq(msg)
		return true
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(msg))
	}
	return false
}

func (m *funcMiddleware) handleReadReq(req *mem.ReadReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newReadRespondEvent(timeToSchedule, m, req)
	m.Engine.Schedule(respondEvent)
}

func (m *funcMiddleware) handleWriteReq(req *mem.WriteReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newWriteRespondEvent(timeToSchedule, m, req)
	m.Engine.Schedule(respondEvent)
}

func (m *funcMiddleware) CurrentTime() sim.VTimeInSec {
	return m.Engine.CurrentTime()
}

func (m *funcMiddleware) setState(state string, rspMessage *mem.ControlMsg) bool {
	ctrlRsp := sim.GeneralRspBuilder{}.
		WithSrc(m.ctrlPort).
		WithDst(rspMessage.Src).
		WithOriginalReq(rspMessage).
		Build()

	err := m.ctrlPort.Send(ctrlRsp)

	if err != nil {
		return false
	}

	m.ctrlPort.RetrieveIncoming()
	m.state = state

	return true
}
