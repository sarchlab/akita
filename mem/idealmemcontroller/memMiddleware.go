package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type memMiddleware struct {
	*Comp
}

func (m *memMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.takeNewReqs() || madeProgress
	madeProgress = m.execute() || madeProgress

	return madeProgress
}

func (m *memMiddleware) takeNewReqs() (madeProgress bool) {
	if m.state != "enable" {
		return false
	}

	for i := 0; i < m.width; i++ {
		msg := m.topPort.RetrieveIncoming()
		if msg == nil {
			return true
		}

		m.inflightBuffer = append(m.inflightBuffer, msg)
	}

	return madeProgress
}

func (m *memMiddleware) execute() bool {
	madeProgress := false

	switch state := m.state; state {
	case "enable", "drain":
		madeProgress = m.handleInflightMemReqs()
	case "pause":
		madeProgress = false
	}

	return madeProgress
}

// updateMemCtrl updates ideal memory controller state.
func (m *memMiddleware) handleInflightMemReqs() bool {
	madeProgress := false
	for i := 0; i < m.width; i++ {
		madeProgress = m.handleMemReqs() || madeProgress
	}

	return madeProgress
}

func (m *memMiddleware) handleMemReqs() bool {
	if len(m.inflightBuffer) == 0 {
		return false
	}

	msg := m.inflightBuffer[0]
	m.inflightBuffer = m.inflightBuffer[1:]

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

func (m *memMiddleware) handleReadReq(req *mem.ReadReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newReadRespondEvent(timeToSchedule, m, req)
	m.Engine.Schedule(respondEvent)
}

func (m *memMiddleware) handleWriteReq(req *mem.WriteReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newWriteRespondEvent(timeToSchedule, m, req)
	m.Engine.Schedule(respondEvent)
}

func (m *memMiddleware) CurrentTime() sim.VTimeInSec {
	return m.Engine.CurrentTime()
}
