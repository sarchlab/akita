package ticking

// Definition defines a ticking component type by its five structs: S is the
// Spec, T the State, R the Resources, P the Ports, and M the Middlewares (see
// the package documentation).
//
// Declare it as a package-level var named Definition in the component's
// package. DefaultSpec must be statically evaluable, a keyed literal with
// constant leaves, because tooling reads it without running the package.
// State and Middlewares may define Initialize(*Component) methods.
// Simulation.Initialize calls them after wiring, State first; omitted methods
// keep zero values. Methods must use pointer receivers and return no value.
type Definition[S, T, R, P, M any] struct {
	// DefaultSpec is the default configuration. The system builder starts
	// from a copy of it. The copy shares any slice with DefaultSpec until
	// Build copies it, so replace a slice field rather than writing into it.
	DefaultSpec S
}

// Builder returns a builder for instances of this component type, starting
// from DefaultSpec.
func (d Definition[S, T, R, P, M]) Builder() Builder[S, T, R, P, M] {
	return Builder[S, T, R, P, M]{spec: d.DefaultSpec}
}
