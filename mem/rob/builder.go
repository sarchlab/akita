package rob

import (
	"github.com/sarchlab/akita/v5/modeling"
)

// DefaultSpec returns a copy of the default reorder-buffer configuration.
// Callers typically take it, tweak the fields they care about, and pass the
// result to WithSpec.
func DefaultSpec() Spec {
	return Definition.DefaultSpec()
}

// Builder constructs reorder-buffer components. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithSimulation. The reorder
// buffer references no shared resources, so no WithResources is exposed. The
// component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort.
type Builder struct {
	spec       Spec
	simulation timing.Simulation
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: Definition.DefaultSpec()}
}

// WithSimulation sets the simulation that owns and registers the built component.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// Build creates a reorder buffer with the given name. It declares the
// component's Top, Bottom, and Control ports and registers the component; the
// port instances are assigned externally after Build with AssignPort (the
// caller chooses the buffer sizes).
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("rob: WithSimulation is required")
	}

	spec := b.spec

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	comp.State = State{}
	comp.AddMiddleware(&middleware{comp: comp})

	Definition.DeclarePorts(comp)

	b.simulation.RegisterComponent(comp)

	return comp
}
