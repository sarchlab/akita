package ticking

import (
	"io"
	"sync"

	"github.com/sarchlab/akita/v5/internal/tickscheduler"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/base"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Component is an instance of a ticking component type, built with modeling.MakeBuilder.
// It adds the clock to what the component models share.
//
// Its exported fields, the five structs, come from the embedded
// ComponentBase:
//
//   - Spec is the configuration given to Build.
//   - State is the instance's mutable runtime data, saved in checkpoints.
//     Only the component itself, its State initializer and middlewares, writes it.
//   - Resources holds the references to shared objects given to Build.
//   - Ports holds the slots the system builder assigns with BindPort.
//   - Middlewares holds the instance's behavior. Every event the instance
//     receives goes to each field in declaration order.
//
// Spec and Resources are fixed after Build; Initialize freezes Ports and
// creates State and Middlewares.
//
// A Component retains HookableBase for application-defined hook points.
// Standard task tracing uses the simulation task tracker.
type Component[S, T, R, P, M any] struct {
	base.ComponentBase[S, T, R, P, M]

	ticks    *tickscheduler.Scheduler
	pipeline []Middleware

	// handling makes the instance handle one event at a time: the parallel
	// engine runs the events of one time concurrently.
	handling sync.Mutex
}

// Handle passes the event, an Event, to every middleware, in the
// declaration order of Middlewares, and schedules the next tick if any of them
// made progress. The instance handles one event at a time.
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
	base.RequireInitialized(&c.ComponentBase)
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

// TickAt makes sure the instance ticks at the first clock edge at or after
// time at, for work that becomes due then; a tick already pending on that
// edge serves the request. The tick carries no data: the instance keeps the
// work in its State, calls TickAt with the due time when it adds the work,
// and handles the work that is due on each tick.
func (c *Component[S, T, R, P, M]) TickAt(at timing.VTimeInPicoSec) {
	c.ticks.TickAt(at)
}

// NotifyRecv wakes the instance when a port receives a message.
func (c *Component[S, T, R, P, M]) NotifyRecv(_ messaging.Port) {
	c.ticks.TickLater()
}

// NotifyPortFree wakes the instance when a port can send again.
func (c *Component[S, T, R, P, M]) NotifyPortFree(_ messaging.Port) {
	c.ticks.TickLater()
}

// SaveCheckpoint writes the instance's State and tick-scheduler guard.
func (c *Component[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	base.RequireInitialized(&c.ComponentBase)
	return modeling.WriteCheckpoint(w, c.State, c.ticks)
}

// LoadCheckpoint restores the State and tick-scheduler guard (best effort).
func (c *Component[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	base.RequireInitialized(&c.ComponentBase)
	return modeling.ReadCheckpoint(r, &c.State, c.ticks)
}

// Name returns the instance name given to Build.
func (c *Component[S, T, R, P, M]) Name() string {
	return c.ComponentBase.Name()
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

// BindPort assigns a declared slot, binds its owner and name, and registers the port.
func (c *Component[S, T, R, P, M]) BindPort(slot string, port messaging.Port) {
	c.ComponentBase.BindPort(slot, port)
}

// TaskTracker returns the simulation's task tracker, or nil when tracking is disabled.
func (c *Component[S, T, R, P, M]) TaskTracker() *tasks.Tracker { return c.ComponentBase.TaskTracker() }
