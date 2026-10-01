package wakeup

import (
	"io"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Component is an instance of a wakeup component type, built from its
// Definition. The embedded ComponentBase holds its five structs; Component
// adds its wakeups.
type Component[S, T, R, P, M any] struct {
	modeling.ComponentBase[S, T, R, P, M]

	wakeups  *Scheduler
	pipeline []modeling.Middleware
}

// Handle passes the event, usually an Event, to every middleware in the
// declaration order of Middlewares. If any of them made progress, the
// instance wakes again at the same time, so it keeps running until no
// middleware has work ready.
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
	if _, ok := e.(Event); ok {
		c.wakeups.Woke(e.Time())
	}

	if modeling.Dispatch(c.pipeline, e) {
		c.wakeups.WakeNow()
	}
}

// WakeAt wakes the instance at time t, for work that becomes ready then.
// Asking for a wakeup at or after one already pending does nothing, so a
// middleware asks again, when it wakes, for the work still waiting.
func (c *Component[S, T, R, P, M]) WakeAt(t timing.VTimeInPicoSec) {
	c.wakeups.WakeAt(t)
}

// NotifyRecv wakes the instance when a port receives a message.
func (c *Component[S, T, R, P, M]) NotifyRecv(_ messaging.Port) {
	c.wakeups.WakeNow()
}

// NotifyPortFree wakes the instance when a port can send again.
func (c *Component[S, T, R, P, M]) NotifyPortFree(_ messaging.Port) {
	c.wakeups.WakeNow()
}

// SaveCheckpoint writes the instance's spec hash, State, and wakeup guard.
func (c *Component[S, T, R, P, M]) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, c.Spec(), c.State, c.wakeups)
}

// LoadCheckpoint restores the State and wakeup guard after verifying that the
// saved spec hash matches this instance's.
func (c *Component[S, T, R, P, M]) LoadCheckpoint(r io.Reader) error {
	return modeling.ReadCheckpoint(r, c.Spec(), &c.State, c.wakeups)
}
