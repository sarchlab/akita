package switches

import (
	"github.com/sarchlab/akita/v4/noc/networking/arbitration"
	"github.com/sarchlab/akita/v4/noc/networking/routing"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// Builder can help building switches
type Builder struct {
	sim          simulation.Simulation
	freq         timing.Freq
	routingTable routing.Table
	arbiter      arbitration.Arbiter
}

func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets the engine that the switch to build uses.
func (b Builder) WithSimulation(sim simulation.Simulation) Builder {
	b.sim = sim
	return b
}

// WithFreq sets the frequency that the switch to build works at.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithArbiter sets the arbiter to be used by the switch to build.
func (b Builder) WithArbiter(arbiter arbitration.Arbiter) Builder {
	b.arbiter = arbiter
	return b
}

// WithRoutingTable sets the routing table to be used by the switch to build.
func (b Builder) WithRoutingTable(rt routing.Table) Builder {
	b.routingTable = rt
	return b
}

// Build creates a new switch
func (b Builder) Build(name string) *Comp {
	b.freqMustNotBeZero()
	b.routingTableMustBeGiven()
	b.arbiterMustBeGiven()

	s := &Comp{}
	s.TickingComponent = modeling.NewTickingComponent(
		name, b.sim.GetEngine(), b.freq, s)
	s.sim = b.sim
	s.routingTable = b.routingTable
	s.arbiter = b.arbiter
	s.portToComplexMapping = make(map[modeling.RemotePort]portComplex)

	middleware := &middleware{Comp: s}
	s.AddMiddleware(middleware)

	return s
}

func (b Builder) freqMustNotBeZero() {
	if b.freq == 0 {
		panic("switch frequency cannot be 0")
	}
}

func (b Builder) routingTableMustBeGiven() {
	if b.routingTable == nil {
		panic("switch requires a routing table to operate")
	}
}

func (b Builder) arbiterMustBeGiven() {
	if b.arbiter == nil {
		panic("switch requires an arbiter to operate")
	}
}
