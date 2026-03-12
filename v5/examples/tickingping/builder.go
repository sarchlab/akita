package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// Builder builds tickingping components.
type Builder struct {
	engine  sim.Engine
	freq    sim.Freq
	outPort sim.Port
}

// MakeBuilder returns a new Builder.
func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets the engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
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
		WithFreq(b.freq).
		WithSpec(Spec{}).
		Build(name)
	comp.SetState(State{})

	comp.AddMiddleware(&sendMW{comp: comp})
	comp.AddMiddleware(&receiveProcessMW{comp: comp})

	b.outPort.SetComponent(comp)
	comp.AddPort("Out", b.outPort)

	return comp
}
