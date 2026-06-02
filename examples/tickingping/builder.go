package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides default configuration for the tickingping component.
var defaultSpec = Spec{
	Freq:              1 * timing.GHz,
	OutPortBufferSize: 4,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds tickingping components. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar. The component
// creates its own Out port.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
}

// MakeBuilder returns a new Builder seeded with the default spec.
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

// Build creates a new tickingping component. It creates the component's Out
// port.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("tickingping: WithRegistrar is required")
	}

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	comp.State = State{}

	comp.AddMiddleware(&sendMW{comp: comp})
	comp.AddMiddleware(&receiveProcessMW{comp: comp})

	outPort := messaging.NewPort(
		comp, b.spec.OutPortBufferSize, b.spec.OutPortBufferSize, name+".Out")
	comp.AddPort("Out", outPort)

	b.registrar.RegisterComponent(comp)

	return comp
}
