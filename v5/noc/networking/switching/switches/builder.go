package switches

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/networking/arbitration"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
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

	spec := Spec{}
	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	infra := &switchInfra{
		comp:      modelComp,
		portIndex: make(map[sim.RemotePort]int),
	}

	rfsMW := &routeForwardSendMW{
		switchInfra:  infra,
		routingTable: b.routingTable,
		arbiter:      b.arbiter,
	}

	rpMW := &receivePipelineMW{
		switchInfra: infra,
	}

	s := &Comp{
		Component: modelComp,
	}

	// Register routeForwardSendMW first (index 0), receivePipelineMW second (index 1).
	// This matches the execution order: sendOut → forward → route → movePipeline → startProcessing
	s.AddMiddleware(rfsMW)
	s.AddMiddleware(rpMW)

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
