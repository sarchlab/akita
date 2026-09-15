package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides default configuration for the tickingping component.
var defaultSpec = Spec{
	Freq: 1 * timing.GHz,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds tickingping components. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation. The component
// declares its "Out" port; the port instance is supplied externally after
// Build with AssignPort (the caller chooses the buffer size).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
}

// MakeBuilder returns a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
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

// Build creates a new tickingping component. It declares the component's "Out"
// port; assign the port instance after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("tickingping: WithSimulation is required")
	}

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithSimulation(b.simulation).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	comp.State = State{}

	comp.AddMiddleware(&sendMW{comp: comp})
	comp.AddMiddleware(&receiveProcessMW{comp: comp})

	comp.DeclarePort("Out")

	b.simulation.RegisterComponent(comp)

	return comp
}
