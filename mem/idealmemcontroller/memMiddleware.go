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

func (m *memMiddleware) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *readRespondEvent:
		return m.handleReadRespondEvent(e)
	case *writeRespondEvent:
		return m.handleWriteRespondEvent(e)
	case sim.TickEvent:
		return m.TickingComponent.Handle(e)
	default:
		log.Panicf("cannot handle event of %s", reflect.TypeOf(e))
	}

	return nil
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
			break
		}

		m.inflightBuffer = append(m.inflightBuffer, msg)
		madeProgress = true
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

func (m *memMiddleware) handleReadRespondEvent(e *readRespondEvent) error {
	now := e.Time()
	req := e.req

	addr := req.Address
	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	data, err := m.Storage.Read(addr, req.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(data).
		Build()

	networkErr := m.topPort.Send(rsp)

	if networkErr != nil {
		retry := newReadRespondEvent(m.Freq.NextTick(now), m, req)
		m.Engine.Schedule(retry)
		return nil
	}

	tracing.TraceReqComplete(req, m)
	m.TickLater()

	return nil
}

func (m *memMiddleware) handleWriteRespondEvent(e *writeRespondEvent) error {
	now := e.Time()
	req := e.req

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	networkErr := m.topPort.Send(rsp)
	if networkErr != nil {
		retry := newWriteRespondEvent(m.Freq.NextTick(now), m, req)
		m.Engine.Schedule(retry)
		return nil
	}

	addr := req.Address

	if m.addressConverter != nil {
		addr = m.addressConverter.ConvertExternalToInternal(addr)
	}

	if req.DirtyMask == nil {
		err := m.Storage.Write(addr, req.Data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		data, err := m.Storage.Read(addr, uint64(len(req.Data)))
		if err != nil {
			panic(err)
		}
		for i := 0; i < len(req.Data); i++ {
			if req.DirtyMask[i] {
				data[i] = req.Data[i]
			}
		}
		err = m.Storage.Write(addr, data)
		if err != nil {
			panic(err)
		}
	}

	tracing.TraceReqComplete(req, m)
	m.TickLater()

	return nil
}
