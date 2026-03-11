package modeling

import (
	"encoding/json"

	"github.com/sarchlab/akita/v5/sim"
)

// Component is a generic component that combines Spec, State, Ports, and
// Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Component uses A-B double buffering for state: 'current' is the read-only
// state visible via [GetState], and 'next' is the writable buffer for the
// upcoming tick. During [Tick], current is deep-copied into next before the
// middleware pipeline runs; after the pipeline completes, next becomes current.
//
// Component embeds [sim.TickingComponent] for tick-based lifecycle management
// and [sim.MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any] struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	spec    S
	current T
	next    T
}

// GetSpec returns the immutable specification.
func (c *Component[S, T]) GetSpec() S {
	return c.spec
}

// GetState returns the current (A-buffer) state. This is the read-only
// snapshot for the current tick.
func (c *Component[S, T]) GetState() T {
	return c.current
}

// GetNextState returns a pointer to the next (B-buffer) state, allowing
// direct mutation of the state that will become current after the tick.
func (c *Component[S, T]) GetNextState() *T {
	return &c.next
}

// SetNextState sets the next (B-buffer) state directly.
func (c *Component[S, T]) SetNextState(state T) {
	c.next = state
}

// SetState sets both current and next buffers. This is used for
// initialization and save/load scenarios where both buffers must agree.
func (c *Component[S, T]) SetState(state T) {
	c.current = state
	c.next = deepCopy(state)
}

// Tick performs the double-buffer cycle:
//  1. Deep-copy current into next.
//  2. Run the middleware pipeline (which may modify next via GetNextState/SetNextState or SetState).
//  3. Swap: current = next.
func (c *Component[S, T]) Tick() bool {
	c.next = deepCopy(c.current)
	madeProgress := c.MiddlewareHolder.Tick()
	c.current = c.next

	return madeProgress
}

// ResetTick resets the TickScheduler so that future TickLater calls can
// schedule new events. This is used after loading state from a checkpoint.
func (c *Component[S, T]) ResetTick() {
	c.TickScheduler.Reset()
}

// ResetAndRestartTick resets the TickScheduler and schedules a new tick.
// This is used after loading state from a checkpoint when the component
// needs to immediately resume ticking.
func (c *Component[S, T]) ResetAndRestartTick() {
	c.TickScheduler.Reset()
	c.TickLater()
}

// deepCopy creates a deep copy of a value using JSON round-trip serialization.
// This works because State types are validated to be JSON-serializable (no
// pointers, interfaces, channels, or functions).
func deepCopy[T any](src T) T {
	data, err := json.Marshal(src)
	if err != nil {
		panic("modeling.deepCopy: marshal failed: " + err.Error())
	}

	var dst T
	if err := json.Unmarshal(data, &dst); err != nil {
		panic("modeling.deepCopy: unmarshal failed: " + err.Error())
	}

	return dst
}
