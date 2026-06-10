package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/control"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for GMMU components.
var defaultSpec = Spec{
	Freq:                1 * timing.GHz,
	Log2PageSize:        12,
	MaxRequestsInFlight: 16,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds GMMU components. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithRegistrar and WithResources. The
// component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort (the caller
// chooses the buffer sizes).
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder creates a new builder seeded with the default spec.
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

// WithResources injects the component's shared resources (e.g. a page table
// shared with other components). If the page table is not set, the component
// builds its own, sized by Spec.Log2PageSize.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build returns a new GMMU. It declares the component's "Top", "Bottom", and
// "Control" ports; assign the port instances after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("gmmu: WithRegistrar is required")
	}

	spec := b.spec

	pt := b.resolvePageTable(name, spec)

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{PageTable: pt}).
		Build(name)

	modelComp.State = State{
		RemoteMemReqs: make(map[uint64]transactionState),
	}

	modelComp.DeclarePort("Top", vmprotocol.Responder)
	modelComp.DeclarePort("Bottom", vmprotocol.Requester)
	modelComp.DeclarePort("Control", control.Responder)

	cMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cMW)

	wMW := &walkMW{
		comp:      modelComp,
		pageTable: pt,
	}
	modelComp.AddMiddleware(wMW)

	rMW := &respondMW{
		comp: modelComp,
	}
	modelComp.AddMiddleware(rMW)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

// resolvePageTable returns the injected page table, or builds a default one
// sized by Spec.Log2PageSize that self-registers with the registrar.
func (b Builder) resolvePageTable(name string, spec Spec) vm.PageTable {
	if b.resources.PageTable != nil {
		return b.resources.PageTable
	}

	return vm.MakePageTableBuilder().
		WithLog2PageSize(spec.Log2PageSize).
		WithSimulation(b.registrar).
		Build(name + ".PageTable")
}
