package addresstranslator

import (
	"github.com/sarchlab/akita/v5/modeling"
)

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return Definition.DefaultSpec()
}

// Builder builds address translator components. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithSimulation and
// WithResources. The component declares its "Top", "Bottom", "Translation",
// and "Control" ports; the port instances are supplied externally after Build
// with AssignPort (the caller chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder creates a new builder seeded with the default spec.
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

// WithResources injects the component's external wiring, i.e. the memory- and
// translation-provider mappers.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build returns a new AddressTranslator. It declares the component's "Top",
// "Bottom", "Translation", and "Control" ports; assign the port instances
// after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("addresstranslator: WithSimulation is required")
	}

	spec := b.spec

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(b.resources).
		Build(name)

	cMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cMW)

	ptMW := &parseTranslateMW{comp: modelComp}
	modelComp.AddMiddleware(ptMW)

	rpMW := &respondPipelineMW{comp: modelComp}
	modelComp.AddMiddleware(rpMW)

	Definition.DeclarePorts(modelComp)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}
