package ping

import "github.com/sarchlab/akita/v4/sim"

type Builder struct {
	name   string
	Engine sim.Engine
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.Engine = engine
	return b
}

func (b Builder) Build(name string) *Comp {
	pingAgent := &Comp{}
	pingAgent.ComponentBase = sim.NewComponentBase(name)
	pingAgent.OutPort = sim.NewLimitNumMsgPort(pingAgent, 4, name+".OutPort")
	pingAgent.Engine = b.Engine

	return pingAgent
}
