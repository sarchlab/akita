package switches

import (
	"github.com/sarchlab/akita/v4/noc/networking/arbitration"
	"github.com/sarchlab/akita/v4/noc/networking/routing"
	"github.com/sarchlab/akita/v4/sim"
)

// Builder can help building switches
type Builder struct {
	engine       sim.Engine
	freq         sim.Freq
	routingTable routing.Table
	arbiter      arbitration.Arbiter
}

func MakeBuilder() Builder {
	return Builder{}
}

// WithEngine sets the engine that the switch to build uses.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the switch to build works at.
func (b Builder) WithFreq(freq sim.Freq) Builder {
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
	b.engineMustBeGiven()
	b.freqMustNotBeZero()
	b.routingTableMustBeGiven()
	b.arbiterMustBeGiven()

	s := &Comp{}
	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.routingTable = b.routingTable
	s.arbiter = b.arbiter
	s.portToComplexMapping = make(map[sim.RemotePort]portComplex)

	middleware := &middleware{Comp: s}
	s.AddMiddleware(middleware)

	return s
}

func (b Builder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine of switch is not given")
	}
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
