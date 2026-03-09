package ping

import "github.com/sarchlab/akita/v5/sim"

type Builder struct {
	Engine  sim.Engine
	outPort sim.Port
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.Engine = engine
	return b
}

func (b Builder) WithOutPort(port sim.Port) Builder {
	b.outPort = port
	return b
}

func (b Builder) Build(name string) *Comp {
	pingAgent := &Comp{}
	pingAgent.ComponentBase = sim.NewComponentBase(name)
	pingAgent.OutPort = b.outPort
	pingAgent.OutPort.SetComponent(pingAgent)
	pingAgent.Engine = b.Engine

	return pingAgent
}
