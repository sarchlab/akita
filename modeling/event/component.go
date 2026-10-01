package event

import (
	"fmt"
	"io"
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/internal/base"
	"github.com/sarchlab/akita/v5/timing"
)

// Component is an instance of an event component type, built from its
// Definition. The embedded ComponentBase holds its five structs; Component
// adds the scheduling of its own events.
type Component[S, T, R, P, M any] struct {
	base.ComponentBase[S, T, R, P, M]

	engine   timing.EventScheduler
	pipeline []modeling.Middleware

	// handling makes the instance handle one event at a time: the parallel
	// engine runs the events of one time concurrently, and an event component
	// often schedules several events for the same time.
	handling sync.Mutex
}

// Handle passes the event to every middleware, in the declaration order of
// Middlewares. The event model does not use the middlewares' progress: an
// event component runs only when an event arrives. The instance handles one
// event at a time.
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
	c.handling.Lock()
	defer c.handling.Unlock()

	base.Dispatch(c.pipeline, e)
}

// NotifyRecv schedules a Recv event for the port at the current time.
func (c *Component[S, T, R, P, M]) NotifyRecv(port messaging.Port) {
	c.Schedule(Recv{
		EventBase: c.MakeEventBase(c.CurrentTime()),
		Port:      port.Name(),
	})
}

// NotifyPortFree schedules a PortFree event for the port at the current time.
func (c *Component[S, T, R, P, M]) NotifyPortFree(port messaging.Port) {
	c.Schedule(PortFree{
		EventBase: c.MakeEventBase(c.CurrentTime()),
		Port:      port.Name(),
	})
}

// MakeEventBase returns the base of a new event for this instance at time t:
// a fresh ID, the time, and this instance as the handler. Embed it in the
// event types the component schedules for itself.
func (c *Component[S, T, R, P, M]) MakeEventBase(
	t timing.VTimeInPicoSec,
) timing.EventBase {
	return timing.MakeEventBase(c.NewID(), t, c.Name())
}

// Schedule schedules an event for this instance. It panics if the event is
// for another handler: a component schedules events only for itself and
// reaches other components through its ports.
func (c *Component[S, T, R, P, M]) Schedule(e timing.Event) {
	if e.HandlerID() != c.Name() {
		panic(fmt.Sprintf(
			"event: component %q cannot schedule an event for %q",
			c.Name(), e.HandlerID()))
	}

	c.engine.Schedule(e)
}

// SaveCheckpoint writes the instance's spec hash and State. Its pending
// events are saved with the engine's event queue.
func (c *Component[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.Spec(), c.State, nil)
}

// LoadCheckpoint restores the State after verifying that the saved spec hash
// matches this instance's.
func (c *Component[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.Spec(), &c.State, nil)
}
