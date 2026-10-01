package ticking

import (
	"io"
	"sync"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/internal/base"
	"github.com/sarchlab/akita/v5/timing"
)

// Component is an instance of a ticking component type, built from its
// Definition. It adds the clock to what the component models share.
//
// Its exported fields come from the embedded ComponentBase:
//
//   - State is the instance's mutable runtime data, saved in checkpoints.
//     Only the component itself, its NewState and middlewares, writes it.
//   - Ports holds the ports the system builder passed to Build, bound to this
//     instance.
//   - Middlewares holds the instance's behavior. Every event the instance
//     receives goes to each field in declaration order.
//
// A Component is hookable (hooking.Hookable): tracing attaches to it with
// AcceptHook.
type Component[S, T, R, P, M any] struct {
	base.ComponentBase[S, T, R, P, M]

	ticks    *Scheduler
	pipeline []modeling.Middleware

	// handling makes the instance handle one event at a time: the parallel
	// engine runs the events of one time concurrently.
	handling sync.Mutex
}

// Handle passes the event, a TickEvent, to every middleware, in the
// declaration order of Middlewares, and schedules the next tick if any of them
// made progress. The instance handles one event at a time.
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
	c.handling.Lock()
	defer c.handling.Unlock()

	if base.Dispatch(c.pipeline, e) {
		c.ticks.TickLater()
	}
}

// TickLater schedules a tick at the next cycle, unless one is already
// scheduled. A component ticks on its own while it makes progress and wakes
// on port activity; TickLater starts one that begins work by itself, such as
// a traffic generator.
func (c *Component[S, T, R, P, M]) TickLater() {
	c.ticks.TickLater()
}

// NotifyRecv wakes the instance when a port receives a message.
func (c *Component[S, T, R, P, M]) NotifyRecv(_ messaging.Port) {
	c.ticks.TickLater()
}

// NotifyPortFree wakes the instance when a port can send again.
func (c *Component[S, T, R, P, M]) NotifyPortFree(_ messaging.Port) {
	c.ticks.TickLater()
}

// SaveCheckpoint writes the instance's spec hash, State, and tick-scheduler
// guard.
func (c *Component[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.Spec(), c.State, c.ticks)
}

// LoadCheckpoint restores the State and tick-scheduler guard after verifying
// that the saved spec hash matches this instance's.
func (c *Component[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.Spec(), &c.State, c.ticks)
}

// Name returns the instance name given to Build.
func (c *Component[S, T, R, P, M]) Name() string {
	return c.ComponentBase.Name()
}

// TypeName returns the component type's name: the import path of the package
// that declares it.
func (c *Component[S, T, R, P, M]) TypeName() string {
	return c.ComponentBase.TypeName()
}

// NewID allocates an ID, unique within the instance's simulation, for a
// message or event the instance creates.
func (c *Component[S, T, R, P, M]) NewID() uint64 {
	return c.ComponentBase.NewID()
}

// CurrentTime returns the simulation's current time.
func (c *Component[S, T, R, P, M]) CurrentTime() timing.VTimeInPicoSec {
	return c.ComponentBase.CurrentTime()
}

// Spec returns the instance's configuration. The returned value is a copy, but
// its slices are the instance's own: treat them as read-only.
func (c *Component[S, T, R, P, M]) Spec() S {
	return c.ComponentBase.Spec()
}

// Resources returns the instance's shared-resource references.
func (c *Component[S, T, R, P, M]) Resources() R {
	return c.ComponentBase.Resources()
}
