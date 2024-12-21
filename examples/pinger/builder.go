package ping

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
)

type Builder struct {
	sim simulation.Simulation
}

func (b Builder) WithSimulation(sim simulation.Simulation) Builder {
	b.sim = sim
	return b
}

func (b Builder) Build(name string) *Comp {
	pingAgent := &Comp{}
	pingAgent.ComponentBase = modeling.NewComponentBase(name)
	pingAgent.engine = b.sim.GetEngine()
	pingAgent.OutPort = modeling.PortBuilder{}.
		WithSimulation(b.sim).
		WithComponent(pingAgent).
		WithIncomingBufCap(4).
		WithOutgoingBufCap(4).
		Build(name + ".OutPort")

	return pingAgent
}
