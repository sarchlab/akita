package modeling

// Component is a generic component that combines Spec, State, Ports, and
// Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Component stores a spec and a single mutable state value.
//
// Component embeds [TickingComponent] for tick-based lifecycle management
// and [MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any] struct {
	*TickingComponent
	MiddlewareHolder

	Spec  S
	State T
}

// Tick runs the middleware pipeline.
func (c *Component[S, T]) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

// StateRef returns a live reference to the component's runtime state. It
// exposes the State field to the simulation's global state manager (it
// satisfies simulation.StateHolder structurally). The returned pointer aliases
// the State field, so reads and writes through it are shared with the
// component.
func (c *Component[S, T]) StateRef() any {
	return &c.State
}
