package ping

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type Builder struct {
	Engine timing.Engine
}

func MakeBuilder() Builder {
	return Builder{}
}

func (b Builder) WithEngine(engine timing.Engine) Builder {
	b.Engine = engine
	return b
}

func (b Builder) Build(name string) *Comp {
	pingAgent := &Comp{}
	pingAgent.ComponentBase = modeling.NewComponentBase(name)
	pingAgent.OutPort = modeling.NewPort(pingAgent, 4, 4, name+".OutPort")
	pingAgent.Engine = b.Engine

	return pingAgent
}
