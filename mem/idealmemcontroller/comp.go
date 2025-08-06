package idealmemcontroller

import (
	"github.com/sarchlab/akita/v4/mem/mem"

	"github.com/sarchlab/akita/v4/sim"
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
	ctrlPort         sim.Port
	Storage          *mem.Storage
	Latency          int
	addressConverter mem.AddressConverter
	currentCmd       *mem.ControlMsg
	width            int
	state            string

	inflightBuffer []sim.Msg
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}
