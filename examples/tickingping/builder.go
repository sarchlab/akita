package tickingping

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type Builder struct {
	engine timing.Engine
	freq   timing.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(engine timing.Engine) Builder {
	b.engine = engine
	return b
}

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

func (b Builder) Build(name string) *Comp {
	tickingPingAgent := &Comp{}

	tickingPingAgent.TickingComponent = modeling.NewTickingComponent(
		name, b.engine, b.freq, tickingPingAgent)

	middleware := &middleware{Comp: tickingPingAgent}
	tickingPingAgent.AddMiddleware(middleware)

	tickingPingAgent.OutPort = modeling.NewPort(
		tickingPingAgent, 4, 4, tickingPingAgent.Name()+".OutPort")

	return tickingPingAgent
}
