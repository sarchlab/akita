package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides default configuration for the tickingping component.
var DefaultSpec = Spec{
	Freq: 1 * sim.GHz,
}

// Builder builds tickingping components.
type Builder struct {
	engine  sim.EventScheduler
	spec    Spec
	outPort sim.Port
}

// MakeBuilder returns a new Builder.
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine.
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithOutPort sets the output port.
func (b Builder) WithOutPort(port sim.Port) Builder {
	b.outPort = port
	return b
}

// Build creates a new tickingping component.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	comp.SetState(State{})

	comp.AddMiddleware(&sendMW{comp: comp})
	comp.AddMiddleware(&receiveProcessMW{comp: comp})

	b.outPort.SetComponent(comp)
	comp.AddPort("Out", b.outPort)

	return comp
}
