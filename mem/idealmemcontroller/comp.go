package idealmemcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"

	"github.com/sarchlab/akita/v4/tracing"
)

type readRespondEvent struct {
	*sim.EventBase
	req *mem.ReadReq
}

func newReadRespondEvent(time sim.VTimeInSec, handler sim.Handler,
	req *mem.ReadReq,
) *readRespondEvent {
	return &readRespondEvent{sim.NewEventBase(time, handler), req}
}

type writeRespondEvent struct {
	*sim.EventBase
	req *mem.WriteReq
}

func newWriteRespondEvent(time sim.VTimeInSec, handler sim.Handler,
	req *mem.WriteReq,
) *writeRespondEvent {
	return &writeRespondEvent{sim.NewEventBase(time, handler), req}
}

// An Comp is an ideal memory controller that can perform read and write
// Ideal memory controller always respond to the request in a fixed number of
// cycles. There is no limitation on the concurrency of this unit.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort          sim.Port
	Storage          *mem.Storage
	Latency          int
	addressConverter mem.AddressConverter

	width int
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

// Handle defines how the Comp handles event
func (c *Comp) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *readRespondEvent:
		return c.handleReadRespondEvent(e)
	case *writeRespondEvent:
		return c.handleWriteRespondEvent(e)
	case sim.TickEvent:
		return c.TickingComponent.Handle(e)
	default:
		log.Panicf("cannot handle event of %s", reflect.TypeOf(e))
	}

	return nil
}

type middleware struct {
	*Comp
}

// Tick updates ideal memory controller state.
func (m *middleware) Tick() bool {
	msg := m.topPort.RetrieveIncoming()
	if msg == nil {
		return false
	}

	tracing.TraceReqReceive(msg, m.Comp)

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

func (m *middleware) handleReadReq(req *mem.ReadReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newReadRespondEvent(timeToSchedule, m.Comp, req)
	m.Engine.Schedule(respondEvent)
}

func (m *middleware) handleWriteReq(req *mem.WriteReq) {
	now := m.CurrentTime()
	timeToSchedule := m.Freq.NCyclesLater(m.Latency, now)
	respondEvent := newWriteRespondEvent(timeToSchedule, m.Comp, req)
	m.Engine.Schedule(respondEvent)
}

func (c *Comp) handleReadRespondEvent(e *readRespondEvent) error {
	now := e.Time()
	req := e.req

	addr := req.Address
	if c.addressConverter != nil {
		addr = c.addressConverter.ConvertExternalToInternal(addr)
	}

	data, err := c.Storage.Read(addr, req.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(c.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(data).
		Build()

	networkErr := c.topPort.Send(rsp)

	if networkErr != nil {
		retry := newReadRespondEvent(c.Freq.NextTick(now), c, req)
		c.Engine.Schedule(retry)

		return nil
	}

	tracing.TraceReqComplete(req, c)
	c.TickLater()

	return nil
}

func (c *Comp) handleWriteRespondEvent(e *writeRespondEvent) error {
	now := e.Time()
	req := e.req

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(c.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	networkErr := c.topPort.Send(rsp)
	if networkErr != nil {
		retry := newWriteRespondEvent(c.Freq.NextTick(now), c, req)
		c.Engine.Schedule(retry)

		return nil
	}

	addr := req.Address

	if c.addressConverter != nil {
		addr = c.addressConverter.ConvertExternalToInternal(addr)
	}

	if req.DirtyMask == nil {
		err := c.Storage.Write(addr, req.Data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		data, err := c.Storage.Read(addr, uint64(len(req.Data)))
		if err != nil {
			panic(err)
		}

		for i := 0; i < len(req.Data); i++ {
			if req.DirtyMask[i] {
				data[i] = req.Data[i]
			}
		}

		err = c.Storage.Write(addr, data)
		if err != nil {
			panic(err)
		}
	}

	tracing.TraceReqComplete(req, c)
	c.TickLater()

	return nil
}

func (c *Comp) CurrentTime() sim.VTimeInSec {
	return c.Engine.CurrentTime()
}
