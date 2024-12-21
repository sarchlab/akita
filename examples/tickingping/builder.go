package tickingping

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type Builder struct {
	sim  simulation.Simulation
	freq timing.Freq
}

func (b Builder) WithSimulation(sim simulation.Simulation) Builder {
	b.sim = sim
	return b
}

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

func (b Builder) Build(name string) *Comp {
	tickingPingAgent := &Comp{}

	tickingPingAgent.TickingComponent = modeling.NewTickingComponent(
		name, b.sim.GetEngine(), b.freq, tickingPingAgent)

	middleware := &middleware{Comp: tickingPingAgent}
	tickingPingAgent.AddMiddleware(middleware)

	tickingPingAgent.OutPort = modeling.PortBuilder{}.
		WithSimulation(b.sim).
		WithComponent(tickingPingAgent).
		WithIncomingBufCap(4).
		WithOutgoingBufCap(4).
		Build(tickingPingAgent.Name() + ".OutPort")

	return tickingPingAgent
}
