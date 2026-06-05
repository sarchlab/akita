package mmuCache

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for mmuCache components.
var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	NumReqPerCycle:        4,
	NumLevels:             5,
	NumBlocks:             1,
	PageSize:              4096,
	LatencyPerLevel:       100,
	Log2PageSize:          12,
	TopPortBufferSize:     16,
	BottomPortBufferSize:  16,
	ControlPortBufferSize: 16,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder builds mmuCache components. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar and WithResources.
// The component creates its own ports.
type Builder struct {
	registrar modeling.Registrar
	spec      Spec
	resources Resources
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
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

// Build creates a new mmuCache. It creates the component's Top, Bottom, and
// Control ports.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("mmuCache: WithRegistrar is required")
	}

	if b.spec.NumBlocks <= 0 {
		panic("mmuCache.Builder: numBlocks must be > 0")
	}

	spec := b.spec

	initialState := State{
		CurrentState:          mmuCacheStateEnable,
		Table:                 initSets(spec.NumLevels, spec.NumBlocks),
		OutstandingBottomReqs: map[uint64]bool{},
	}

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(b.resources).
		Build(name)
	modelComp.State = initialState

	topPort := messaging.NewPort(
		modelComp, spec.TopPortBufferSize, spec.TopPortBufferSize, name+".Top")
	modelComp.AddPort("Top", topPort)

	bottomPort := messaging.NewPort(
		modelComp, spec.BottomPortBufferSize, spec.BottomPortBufferSize,
		name+".Bottom")
	modelComp.AddPort("Bottom", bottomPort)

	controlPort := messaging.NewPort(
		modelComp, spec.ControlPortBufferSize, spec.ControlPortBufferSize,
		name+".Control")
	modelComp.AddPort("Control", controlPort)

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	cacheMW := &mmuCacheMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cacheMW)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}
