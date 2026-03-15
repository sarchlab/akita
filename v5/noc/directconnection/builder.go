package directconnection

import "github.com/sarchlab/akita/v5/sim"

// Builder can help building directconnection.
type Builder struct {
	engine sim.EventScheduler
	freq   sim.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(e sim.EventScheduler) Builder {
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
