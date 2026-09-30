package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Builder builds MMU components. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithSimulation and WithResources. The
// component declares its "Top" and "Control" ports; the port instances are
// supplied externally after Build with AssignPort (the caller chooses the
// buffer sizes).
type Builder struct {
	simulation timing.Simulation
	spec       Spec
	resources  Resources
}

// MakeBuilder creates a new builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: Definition.DefaultSpec,
	}
}

// WithSimulation sets the simulation that owns and registers the built component.
func (b Builder) WithSimulation(sim timing.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithSpec sets the entire configuration. Start from Definition.DefaultSpec and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the component's shared resources, such as a page table
// shared with other components. If no page table is supplied, the component
// builds its own.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build returns a newly created MMU component. It declares the component's
// "Top" and "Control" ports; assign the port instances after Build with
// AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("mmu: WithSimulation is required")
	}

	spec := b.spec

	pt := b.resolvePageTable(name, spec)

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithDefinition(Definition).
		WithResources(Resources{PageTable: pt}).
		Build(name)

	cmw := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cmw)

	tmw := &translationMW{comp: modelComp}
	modelComp.AddMiddleware(tmw)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}

// resolvePageTable returns the injected page table, or builds a default one
// sized by Spec.Log2PageSize that self-registers with the simulation.
func (b Builder) resolvePageTable(name string, spec Spec) vm.PageTable {
	if b.resources.PageTable != nil {
		validatePageTablePageSize(b.resources.PageTable, spec.Log2PageSize)
		return b.resources.PageTable
	}

	return vm.MakePageTableBuilder().
		WithLog2PageSize(spec.Log2PageSize).
		WithSimulation(b.simulation).
		Build(name + ".PageTable")
}

// validatePageTablePageSize checks if the provided page table's page size is
// consistent with the MMU's log2PageSize configuration.
func validatePageTablePageSize(pt vm.PageTable, log2PageSize uint64) {
	if pageTableInterface, ok := pt.(pageTable); ok {
		pageTableLog2PageSize := pageTableInterface.GetLog2PageSize()
		if pageTableLog2PageSize != log2PageSize {
			panic("page table page size does not match MMU page size")
		}
	}
}
