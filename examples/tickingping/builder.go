package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// DefaultSpec provides default configuration for the tickingping component.
var DefaultSpec = Spec{
	Freq: 1 * timing.GHz,
}

// Builder builds tickingping components.
type Builder struct {
	engine    timing.EventScheduler
	registrar modeling.Registrar
	spec      Spec
	outPort   messaging.Port
}

// MakeBuilder returns a new Builder.
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine.
func (b Builder) WithEngine(engine timing.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithSimulation wires the builder to a simulation. It sources the engine from
// the simulation and registers the built component with it.
func (b Builder) WithSimulation(sim modeling.Registrar) Builder {
	b.registrar = sim
	b.engine = sim.GetEngine()
	return b
}

// WithFreq sets the frequency.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithOutPort sets the output port.
func (b Builder) WithOutPort(port messaging.Port) Builder {
	b.outPort = port
	return b
}

// Build creates a new tickingping component.
func (b Builder) Build(name string) *Comp {
	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	comp.State = State{}

	comp.AddMiddleware(&sendMW{comp: comp})
	comp.AddMiddleware(&receiveProcessMW{comp: comp})

	b.outPort.SetComponent(comp)
	comp.AddPort("Out", b.outPort)

	if b.registrar != nil {
		b.registrar.RegisterComponent(comp)
	}

	return comp
}
