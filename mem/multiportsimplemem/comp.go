package multiportsimplemem

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// Comp is a simple ticking-based memory component that supports multiple top
// ports and processes requests with fixed latency and limited concurrency.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPorts []sim.Port

	Storage          *mem.Storage
	Latency          int
	ConcurrentSlots  int
	addressConverter mem.AddressConverter
	msgArrivalOrder  map[string]uint64
	nextArrivalOrder uint64
	nextServiceIndex uint64
	waitingRequests  []*pendingRequest
	activeRequests   []*activeRequest
	pendingResponses []*pendingResponse
}

// Tick triggers the middleware chain for the component.
func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

func (c *Comp) recordArrival(req mem.AccessReq) {
	if c.msgArrivalOrder == nil {
		c.msgArrivalOrder = make(map[string]uint64)
	}

	msgID := req.Meta().ID

	if _, found := c.msgArrivalOrder[msgID]; found {
		return
	}

	c.msgArrivalOrder[msgID] = c.nextArrivalOrder
	c.nextArrivalOrder++
}

func (c *Comp) arrivalOrderOf(req mem.AccessReq) uint64 {
	if c.msgArrivalOrder == nil {
		c.msgArrivalOrder = make(map[string]uint64)
	}

	if order, ok := c.msgArrivalOrder[req.Meta().ID]; ok {
		return order
	}

	order := c.nextArrivalOrder
	c.nextArrivalOrder++
	c.msgArrivalOrder[req.Meta().ID] = order

	return order
}

func (c *Comp) removeArrivalRecord(req mem.AccessReq) {
	if c.msgArrivalOrder == nil {
		return
	}

	delete(c.msgArrivalOrder, req.Meta().ID)
}

// Ports returns all the ports that can be connected to agents.
func (c *Comp) Ports() []sim.Port {
	return c.topPorts
}

// Port returns the i-th top port.
func (c *Comp) Port(i int) sim.Port {
	return c.topPorts[i]
}
