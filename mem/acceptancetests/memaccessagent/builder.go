// builder.go
package memaccessagent

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for the memory access agent.
var defaultSpec = Spec{
	Freq:              1 * timing.GHz,
	MaxAddress:        1024 * 1024,
	WriteLeft:         1000,
	ReadLeft:          1000,
	MemPortBufferSize: 1,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder constructs MemAccessAgent instances. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component creates its own ports.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
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

// Build creates a new MemAccessAgent with the given name. It creates the
// agent's Mem port internally.
func (b Builder) Build(name string) *MemAccessAgent {
	if b.registrar == nil {
		panic("memaccessagent: WithRegistrar is required")
	}

	spec := b.spec

	initialState := State{
		WriteLeft:       spec.WriteLeft,
		ReadLeft:        spec.ReadLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]mem.ReadReq),
		PendingWriteReq: make(map[uint64]mem.WriteReq),
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

	mw := &agentMiddleware{agent: agent}
	modelComp.AddMiddleware(mw)

	memPort := messaging.NewPort(
		agent, spec.MemPortBufferSize, spec.MemPortBufferSize, name+".Mem")
	modelComp.AddPort("Mem", memPort)

	b.registrar.RegisterComponent(agent)

	return agent
}
