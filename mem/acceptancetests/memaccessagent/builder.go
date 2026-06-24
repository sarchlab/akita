// builder.go
package memaccessagent

import (
	"math/rand"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for the memory access agent.
var defaultSpec = Spec{
	Freq:       1 * timing.GHz,
	MaxAddress: 1024 * 1024,
	WriteLeft:  1000,
	ReadLeft:   1000,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder constructs MemAccessAgent instances. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component declares its "Mem" port; the port instance is
// supplied externally after Build with AssignPort (the caller chooses the
// buffer size).
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
	randSeed  *int64
}

// MakeBuilder returns a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
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

// WithResources injects the component's external wiring, notably the downstream
// LowModule port that memory requests are sent to. The same port can also be
// assigned to the public LowModule field after Build when construction ordering
// requires it.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// WithRandSeed makes the agent draw from a private RNG seeded with the given
// value, so a run is reproducible from the seed. Without it, the agent uses the
// global (auto-seeded) random source. This is the supported way to get a
// deterministic access stream: math/rand.Seed has been a no-op since Go 1.24,
// so seeding the global source no longer works.
func (b Builder) WithRandSeed(seed int64) Builder {
	b.randSeed = &seed
	return b
}

// Build creates a new MemAccessAgent with the given name. It declares the
// agent's "Mem" port; assign the port instance after Build with AssignPort.
func (b Builder) Build(name string) *MemAccessAgent {
	if b.registrar == nil {
		panic("memaccessagent: WithRegistrar is required")
	}

	spec := b.spec

	initialState := State{
		WriteLeft:       spec.WriteLeft,
		ReadLeft:        spec.ReadLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]memprotocol.ReadReq),
		PendingWriteReq: make(map[uint64]memprotocol.WriteReq),
	}

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.State = initialState

	agent := &MemAccessAgent{
		Component: modelComp,
	}

	if b.resources.LowModule != nil {
		agent.LowModule = b.resources.LowModule
	}

	if b.randSeed != nil {
		agent.rng = rand.New(rand.NewSource(*b.randSeed))
	}

	mw := &agentMiddleware{agent: agent}
	modelComp.AddMiddleware(mw)

	modelComp.DeclarePort("Mem", memprotocol.Requester)

	b.registrar.RegisterComponent(agent)

	return agent
}
