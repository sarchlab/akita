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

// Build creates a new Comp.
func (b Builder) Build(name string) *Comp {
	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(Spec{}).
		Build(name)
	modelComp.SetState(State{})

	c := &Comp{
		Component: modelComp,
	}

	mw := &middleware{Comp: c}
	c.AddMiddleware(mw)

	c.OutPort = b.outPort
	c.OutPort.SetComponent(c)
	c.AddPort("Out", c.OutPort)

	return c
}
