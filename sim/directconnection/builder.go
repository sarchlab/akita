package directconnection

import "github.com/sarchlab/akita/v3/sim"

// Builder can help building directconnection.
type Builder struct {
	engine sim.Engine
	freq   sim.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(e sim.Engine) Builder {
	b.engine = e
	return b
}

func (b Builder) WithFreq(f sim.Freq) Builder {
	b.freq = f
	return b
}

func (b Builder) Build(name string) *Comp {
	c := new(Comp)
	c.TickingComponent = sim.NewSecondaryTickingComponent(name, b.engine, b.freq, c)
	c.ends = make(map[sim.Port]*directConnectionEnd)
	return c
}
