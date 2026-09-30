package ticking

import (
	"io"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Component is an instance of a ticking component type, built from its
// Definition. The embedded ComponentBase holds its five structs; Component
// adds the clock.
type Component[S, T, R, P, M any] struct {
	modeling.ComponentBase[S, T, R, P, M]

	ticks    *modeling.TickScheduler
	pipeline []modeling.Middleware
}

// Handle passes the event to every middleware, in the declaration order of
// Middlewares, and schedules the next tick if any of them made progress. The
// event is usually a tick; it can also be an event the component scheduled
// for itself.
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
	if modeling.Dispatch(c.pipeline, e) {
		c.ticks.TickLater()
	}
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
