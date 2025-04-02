package tickingping

import "github.com/sarchlab/akita/v4/sim"

type Builder struct {
	engine sim.Engine
	freq   sim.Freq
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

func (b Builder) Build(name string) *Comp {
	tickingPingAgent := &Comp{}

	tickingPingAgent.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, tickingPingAgent)

	middleware := &middleware{Comp: tickingPingAgent}
	tickingPingAgent.AddMiddleware(middleware)

	tickingPingAgent.OutPort = sim.NewPort(
		tickingPingAgent, 4, 4, tickingPingAgent.Name()+".OutPort")

	return tickingPingAgent
}
