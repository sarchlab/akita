package datamover

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides default configuration for the data mover.
var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	CtrlPortBufferSize:    16,
	InsidePortBufferSize:  16,
	OutsidePortBufferSize: 16,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds StreamingDataMover components. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component creates its own ports.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder creates a new Builder seeded with the default spec.
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

// WithResources injects the component's wiring (the inside/outside address
// mappers). The data mover owns no storage of its own.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new StreamingDataMover. It creates the component's Control,
// Inside, and Outside ports.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("datamover: WithRegistrar is required")
	}

	spec := b.resolveSpec()
	initialState := State{}

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.State = initialState

	ctrlMW := &ctrlParseMW{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	dataMW := &dataTransferMW{comp: modelComp}
	modelComp.AddMiddleware(dataMW)

	ctrlPort := messaging.NewPort(
		modelComp, spec.CtrlPortBufferSize, spec.CtrlPortBufferSize,
		name+".Control")
	modelComp.AddPort("Control", ctrlPort)

	insidePort := messaging.NewPort(
		modelComp, spec.InsidePortBufferSize, spec.InsidePortBufferSize,
		name+".Inside")
	modelComp.AddPort("Inside", insidePort)

	outsidePort := messaging.NewPort(
		modelComp, spec.OutsidePortBufferSize, spec.OutsidePortBufferSize,
		name+".Outside")
	modelComp.AddPort("Outside", outsidePort)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}

// resolveSpec produces the final Spec used by the component. Any address mapper
// injected through Resources takes precedence and is decomposed into the flat
// mapper fields read at Tick time; otherwise the Spec's own mapper fields are
// used as-is.
func (b Builder) resolveSpec() Spec {
	spec := b.spec

	if b.resources.InsideMapper != nil {
		inlineMapper(b.resources.InsideMapper,
			&spec.InsideMapperKind,
			&spec.InsideMapperPorts,
			&spec.InsideMapperInterleavingSize)
	}

	if b.resources.OutsideMapper != nil {
		inlineMapper(b.resources.OutsideMapper,
			&spec.OutsideMapperKind,
			&spec.OutsideMapperPorts,
			&spec.OutsideMapperInterleavingSize)
	}

	return spec
}

// inlineMapper converts an AddressToPortMapper into serializable Spec fields.
func inlineMapper(
	mapper mem.AddressToPortMapper,
	kind *string,
	ports *[]messaging.RemotePort,
	interleavingSize *uint64,
) {
	switch m := mapper.(type) {
	case *mem.SinglePortMapper:
		*kind = "single"
		*ports = []messaging.RemotePort{m.Port}
		*interleavingSize = 0
	case *mem.InterleavedAddressPortMapper:
		*kind = "interleaved"
		*ports = make([]messaging.RemotePort, len(m.LowModules))
		copy(*ports, m.LowModules)
		*interleavingSize = m.InterleavingSize
	default:
		panic("unsupported mapper type for inline conversion")
	}
}
