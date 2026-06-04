package rob

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	BufferSize:            128,
	NumReqPerCycle:        4,
	TopPortBufferSize:     8,
	BottomPortBufferSize:  8,
	ControlPortBufferSize: 1,
}

// DefaultSpec returns a copy of the default reorder-buffer configuration.
// Callers typically take it, tweak the fields they care about, and pass the
// result to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder constructs reorder-buffer components. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar. The reorder
// buffer references no shared resources, so no WithResources is exposed.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// Build creates a reorder buffer with the given name. It creates the
// component's Top, Bottom, and Control ports and registers the component.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("rob: WithRegistrar is required")
	}

	spec := b.spec

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	comp.State = State{}
	comp.AddMiddleware(&middleware{comp: comp})

	topPort := messaging.NewPort(
		comp, spec.TopPortBufferSize, spec.TopPortBufferSize, name+".Top")
	comp.AddPort("Top", topPort)

	bottomPort := messaging.NewPort(
		comp, spec.BottomPortBufferSize, spec.BottomPortBufferSize,
		name+".Bottom")
	comp.AddPort("Bottom", bottomPort)

	ctrlPort := messaging.NewPort(
		comp, spec.ControlPortBufferSize, spec.ControlPortBufferSize,
		name+".Control")
	comp.AddPort("Control", ctrlPort)

	b.registrar.RegisterComponent(comp)

	return comp
}
