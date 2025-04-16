package directconnection

import "github.com/sarchlab/akita/v4/sim"

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
	c := &Comp{
		ports: ports{
			ports:   make([]sim.Port, 0),
			portMap: make(map[sim.RemotePort]int),
		},
	}
	c.TickingComponent = sim.NewSecondaryTickingComponent(
		name, b.engine, b.freq, c)

	middleware := &middleware{
		Comp: c,
	}
	c.AddMiddleware(middleware)

	return c
}
