package directconnection

import (
	"github.com/sarchlab/akita/v4/sim/model"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// Builder can help building directconnection.
type Builder struct {
	engine timing.Engine
	freq   timing.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(e timing.Engine) Builder {
	b.engine = e
	return b
}

func (b Builder) WithFreq(f timing.Freq) Builder {
	b.freq = f
	return b
}

func (b Builder) Build(name string) *Comp {
	c := &Comp{
		ports: ports{
			ports:   make([]model.Port, 0),
			portMap: make(map[model.RemotePort]int),
		},
	}
	c.TickingComponent = model.NewSecondaryTickingComponent(
		name, b.engine, b.freq, c)

	middleware := &middleware{
		Comp: c,
	}
	c.AddMiddleware(middleware)

	return c
}
