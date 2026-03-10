package modeling

import "github.com/sarchlab/akita/v5/sim"

// Component is a generic component that combines Spec, State, Ports, and
// Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Component embeds [sim.TickingComponent] for tick-based lifecycle management
// and [sim.MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any] struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	spec  S
	state T
}

// GetSpec returns the immutable specification.
func (c *Component[S, T]) GetSpec() S {
	return c.spec
}

// GetState returns the current state.
func (c *Component[S, T]) GetState() T {
	return c.state
}

// SetState sets the component state (for restore/snapshot).
func (c *Component[S, T]) SetState(state T) {
	c.state = state
}

// Tick delegates to the middleware pipeline.
func (c *Component[S, T]) Tick() bool {
	return c.MiddlewareHolder.Tick()
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
