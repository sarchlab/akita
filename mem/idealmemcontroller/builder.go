package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling"
)

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return Definition.DefaultSpec()
}

// Builder builds ideal memory controller components. Configuration is supplied
// as a whole through WithSpec; wiring is supplied through WithSimulation and
// WithResources. The component declares its "Top" and "Control" ports; the
// port instances are supplied externally after Build with AssignPort (the
// caller chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder returns a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: Definition.DefaultSpec()}
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

// WithResources injects the component's shared resources (e.g. a storage shared
// with other components). If not set, the component builds its own, sized by
// Spec.Capacity.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new Comp. It declares the component's "Top" and "Control"
// ports; assign the port instances after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("idealmemcontroller: WithSimulation is required")
	}

	spec := b.spec
	spec.StorageRef = name + ".Storage"

	storage := b.resolveStorage(name, spec)

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{Storage: storage}).
		Build(name)
	modelComp.State = State{ControlState: memcontrolprotocol.StateEnabled}

	modelComp.AddMiddleware(&ctrlMiddleware{comp: modelComp})
	modelComp.AddMiddleware(&memMiddleware{comp: modelComp})

	Definition.DeclarePorts(modelComp)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}

// resolveStorage returns the injected storage, or builds a default one sized by
// Spec.Capacity that self-registers with the simulation.
func (b Builder) resolveStorage(name string, spec Spec) *mem.Storage {
	if b.resources.Storage != nil {
		return b.resources.Storage
	}

	return mem.MakeStorageBuilder().
		WithCapacity(spec.Capacity).
		WithSimulation(b.simulation).
		Build(name + ".Storage")
}
