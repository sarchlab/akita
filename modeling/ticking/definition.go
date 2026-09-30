package ticking

// Definition defines a ticking component type by its five structs: S is the
// Spec, T the State, R the Resources, P the Ports, and M the Middlewares (see
// the package documentation).
//
// Declare it as a package-level var named Definition in the component's
// package. DefaultSpec must be statically evaluable, a keyed literal with
// constant leaves, because tooling reads it without running the package.
// NewState and NewMiddlewares must name functions.
type Definition[S, T, R, P, M any] struct {
	// DefaultSpec is the default configuration. Spec fields are scalars, so
	// reading it yields an independent copy the system builder can change.
	DefaultSpec S

	// NewState returns the initial State of the instance, which may depend on
	// its name, Spec, Resources, and Ports. Build calls it once, after the
	// ports are bound and before the middlewares exist. Omit it to start from
	// the zero State.
	NewState func(c *Component[S, T, R, P, M]) T

	// NewMiddlewares returns the instance's middlewares. Build calls it once,
	// after the ports are bound and the State is set.
	NewMiddlewares func(c *Component[S, T, R, P, M]) M
}

// Builder returns a builder for instances of this component type, starting
// from DefaultSpec.
func (d Definition[S, T, R, P, M]) Builder() Builder[S, T, R, P, M] {
	return Builder[S, T, R, P, M]{def: d, spec: d.DefaultSpec}
}
