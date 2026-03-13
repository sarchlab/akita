package switches

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides the default configuration for switch components.
var DefaultSpec = Spec{
	Freq: 1 * sim.GHz,
}

// Builder can help building switches
type Builder struct {
	engine       sim.Engine
	spec         Spec
	routingTable routing.Table
}

func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine that the switch to build uses.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the switch to build works at.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
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

	spec := b.spec
	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)

	portIndex := make(map[sim.RemotePort]int)

	rfsMW := &routeForwardSendMW{
		comp:         modelComp,
		portIndex:    portIndex,
		routingTable: b.routingTable,
	}

	rpMW := &receivePipelineMW{
		comp:      modelComp,
		portIndex: portIndex,
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
	if b.spec.Freq == 0 {
		panic("switch frequency cannot be 0")
	}
}

func (b Builder) routingTableMustBeGiven() {
	if b.routingTable == nil {
		panic("switch requires a routing table to operate")
	}
}
