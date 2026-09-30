package datamover

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Builder builds StreamingDataMover components. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithSimulation and
// WithResources. The component declares its "Top", "Inside", "Outside", and
// "Control" ports; the port instances are supplied externally after Build with
// AssignPort (the caller chooses the buffer sizes).
type Builder struct {
	spec       Spec
	simulation timing.Simulation
	resources  Resources
}

// MakeBuilder creates a new Builder seeded with the default spec.
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

// WithResources injects the component's wiring (the inside/outside address
// mappers). The data mover owns no storage of its own.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new StreamingDataMover. It declares the component's "Top",
// "Inside", "Outside", and "Control" ports; assign the port instances after
// Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.simulation == nil {
		panic("datamover: WithSimulation is required")
	}

	spec := b.spec
	initialState := State{
		InsideMapper:  flattenMapper(b.resources.InsideMapper),
		OutsideMapper: flattenMapper(b.resources.OutsideMapper),
	}

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithSimulation(b.simulation).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithDefinition(Definition).
		Build(name)
	modelComp.State = initialState

	cMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(cMW)

	parseMW := &ctrlParseMW{comp: modelComp}
	modelComp.AddMiddleware(parseMW)

	dataMW := &dataTransferMW{comp: modelComp}
	modelComp.AddMiddleware(dataMW)

	b.simulation.RegisterComponent(modelComp)

	return modelComp
}

// flattenMapper converts an AddressToPortMapper into the serializable
// portMapping kept in State. A nil mapper yields an empty mapping.
func flattenMapper(mapper mem.AddressToPortMapper) portMapping {
	switch m := mapper.(type) {
	case nil:
		return portMapping{}
	case *mem.SinglePortMapper:
		return portMapping{
			Kind:  "single",
			Ports: []messaging.RemotePort{m.Port},
		}
	case *mem.InterleavedAddressPortMapper:
		ports := make([]messaging.RemotePort, len(m.LowModules))
		copy(ports, m.LowModules)

		return portMapping{
			Kind:             "interleaved",
			Ports:            ports,
			InterleavingSize: m.InterleavingSize,
		}
	default:
		panic("unsupported mapper type for inline conversion")
	}
}
