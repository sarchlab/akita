package modeling

// None is the resource type for components that reference no shared resources.
// It is a zero-size sentinel used as the third type argument of Component:
// Component[Spec, State, None].
type None struct{}

// Component is a generic component that combines Spec, State, Resources, Ports,
// and Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
// R is the Resources type (references to shared resources; None when unused).
//
// Component embeds [TickingComponent] for tick-based lifecycle management
// and [MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any, R any] struct {
	*TickingComponent
	MiddlewareHolder

	Spec      S
	State     T
	Resources R
}

// Tick runs the middleware pipeline.
func (c *Component[S, T, R]) Tick() bool {
	return c.MiddlewareHolder.Tick()
}
