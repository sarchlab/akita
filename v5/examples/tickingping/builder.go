package tickingping

import "github.com/sarchlab/akita/v5/sim"

type Builder struct {
	engine  sim.Engine
	freq    sim.Freq
	outPort sim.Port
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

func (b Builder) WithOutPort(port sim.Port) Builder {
	b.outPort = port
	return b
}

func (b Builder) Build(name string) *Comp {
	tickingPingAgent := &Comp{}

	tickingPingAgent.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, tickingPingAgent)

	middleware := &middleware{Comp: tickingPingAgent}
	tickingPingAgent.AddMiddleware(middleware)

	tickingPingAgent.OutPort = b.outPort
	tickingPingAgent.OutPort.SetComponent(tickingPingAgent)

	return tickingPingAgent
}
