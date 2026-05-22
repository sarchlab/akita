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
