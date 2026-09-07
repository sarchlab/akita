package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for mmuCache components.
var defaultSpec = Spec{
	Freq:            1 * timing.GHz,
	NumReqPerCycle:  4,
	NumLevels:       5,
	NumBlocks:       1,
	PageSize:        4096,
	LatencyPerLevel: 100,
	Log2PageSize:    12,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder builds mmuCache components. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation and WithResources.
// The component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort (the caller
// chooses the buffer sizes).
type Builder struct {
	simulation timing.Simulation
	spec       Spec
	resources  Resources
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithSimulation sets the simulation that owns and registers the built component.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the component's external wiring (the low-module and
// up-module remote ports).
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new mmuCache. It declares the component's "Top", "Bottom", and
// "Control" ports; assign the port instances after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("mmuCache: WithSimulation is required")
	}

	if b.spec.NumBlocks <= 0 {
		panic("mmuCache.Builder: numBlocks must be > 0")
	}

	spec := b.spec

	initialState := State{
		CurrentState:          mmuCacheStateEnable,
		Table:                 initSets(spec.NumLevels, spec.NumBlocks),
		OutstandingBottomReqs: map[uint64]bool{},
		InflightReqs:          map[uint64]inflightReqState{},
	}

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(b.resources).
		Build(name)
	modelComp.State = initialState

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	cacheMW := &mmuCacheMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cacheMW)

	modelComp.DeclarePort("Top", vmprotocol.Responder)
	modelComp.DeclarePort("Bottom", vmprotocol.Requester)
	modelComp.DeclarePort("Control", memcontrolprotocol.Responder)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}
