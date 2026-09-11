package tlb

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
)

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return Definition.DefaultSpec()
}

// A Builder can build TLBs. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithSimulation and WithResources. The
// component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort (the caller
// chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: Definition.DefaultSpec(),
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

// WithResources injects the component's external wiring, in particular the
// translation provider mapper used to locate the remote port that serves the
// translation for a given virtual address.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a new TLB. It declares the component's Top, Bottom, and Control
// ports and registers the component; the port instances are assigned externally
// after Build with AssignPort (the caller chooses the buffer sizes).
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("tlb: WithSimulation is required")
	}

	spec := b.spec
	if spec.Log2PageSize != 0 {
		spec.PageSize = 1 << spec.Log2PageSize
	}
	spec.PipelineWidth = spec.NumReqPerCycle

	initialState := State{
		TLBState: tlbStateEnable,
		Sets:     initSets(spec.NumSets, spec.NumWays),
		Pipeline: queueing.NewPipeline[pipelineTLBReqState](
			spec.PipelineWidth,
			spec.Latency,
		),
		BufferItems: queueing.NewBuffer[pipelineTLBReqState](
			name+".BufferItems",
			spec.PipelineWidth,
		),
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

	tlbMW := &tlbMiddleware{comp: modelComp}
	modelComp.AddMiddleware(tlbMW)

	Definition.DeclarePorts(modelComp)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}
