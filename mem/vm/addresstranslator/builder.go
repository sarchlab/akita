package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// DefaultSpec provides the default configuration for address translators.
var DefaultSpec = Spec{
	Freq:           1 * timing.GHz,
	NumReqPerCycle: 4,
	Log2PageSize:   12,
	DeviceID:       1,
}

// A Builder can create address translators
type Builder struct {
	engine    timing.EventScheduler
	registrar modeling.Registrar
	spec      Spec

	topPort         messaging.Port
	bottomPort      messaging.Port
	translationPort messaging.Port
	ctrlPort        messaging.Port

	memPortMapper             mem.AddressToPortMapper
	memPortMapperType         string
	memRemotePorts            []messaging.RemotePort
	translationPortMapper     mem.AddressToPortMapper
	translationPortMapperType string
	translationRemotePorts    []messaging.RemotePort
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine to be used by the address translators
func (b Builder) WithEngine(engine timing.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithSimulation wires the builder to a simulation. It sources the engine from
// the simulation and registers the built component with it.
func (b Builder) WithSimulation(sim modeling.Registrar) Builder {
	b.registrar = sim
	b.engine = sim.GetEngine()
	return b
}

// WithFreq sets the frequency of the address translators
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNumReqPerCycle sets the number of request the address translators can
// process in each cycle.
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.spec.NumReqPerCycle = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.spec.Log2PageSize = n
	return b
}

// WithDeviceID sets the GPU ID that the address translator belongs to
func (b Builder) WithDeviceID(n uint64) Builder {
	b.spec.DeviceID = n
	return b
}

// WithTopPort sets the top port of the address translator
func (b Builder) WithTopPort(p messaging.Port) Builder {
	b.topPort = p
	return b
}

// WithBottomPort sets the bottom port of the address translator
func (b Builder) WithBottomPort(p messaging.Port) Builder {
	b.bottomPort = p
	return b
}

// WithTranslationPort sets the translation port of the address translator
func (b Builder) WithTranslationPort(p messaging.Port) Builder {
	b.translationPort = p
	return b
}

// WithCtrlPort sets the port of the component that can send ctrl reqs to AT
func (b Builder) WithCtrlPort(p messaging.Port) Builder {
	b.ctrlPort = p
	return b
}

// WithMemoryProviderMapper sets the low modules finder that can tell the
// address translators where to send the memory access request to.
func (b Builder) WithMemoryProviderMapper(f mem.AddressToPortMapper) Builder {
	b.memPortMapper = f
	return b
}

// WithMemoryProviderType sets the type of the memory provider mapper. The
// mapper can find the remote port that can provide the memory service according
// to the virtual address. The type can be "single" or "interleaved".
func (b Builder) WithMemoryProviderType(t string) Builder {
	b.memPortMapperType = t
	return b
}

// WithMemoryProviders registers the remote ports that handle memory access
// requests.
//
// Use together with `WithMemoryProviderType` to control request distribution:
//   - "single": exactly one port must be provided.
//   - "interleaved": the number of ports must be a power of two; requests are
//     interleaved at page granularity (4 KiB by default).
func (b Builder) WithMemoryProviders(ports ...messaging.RemotePort) Builder {
	b.memRemotePorts = ports
	return b
}

// WithTranslationProviderMapper sets the mapper that can find the remote port
// that can provide the translation service according to the virtual address.
func (b Builder) WithTranslationProviderMapper(
	table mem.AddressToPortMapper,
) Builder {
	b.translationPortMapper = table
	return b
}

// WithTranslationProviderMapperType sets the type of the translation provider
// mapper. The mapper can find the remote port that can provide the translation
// service according to the virtual address. The type can be "single" or
// "interleaved".
func (b Builder) WithTranslationProviderMapperType(t string) Builder {
	b.translationPortMapperType = t
	return b
}

// WithTranslationProviders registers the remote ports that handle address
// translation requests.
//
// Use together with `WithTranslationProviderMapperType` to control request
// distribution:
//   - "single": exactly one port must be provided.
//   - "interleaved": the number of ports must be a power of two; requests are
//     interleaved at page granularity (4 KiB by default).
func (b Builder) WithTranslationProviders(ports ...messaging.RemotePort) Builder {
	b.translationRemotePorts = ports
	return b
}

// Build returns a new AddressTranslator
func (b Builder) Build(name string) *Comp {
	spec := b.spec

	b.populateMemMapperSpec(&spec)
	b.populateTransMapperSpec(&spec)

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)

	ptMW := &parseTranslateMW{comp: modelComp}
	modelComp.AddMiddleware(ptMW)

	rpMW := &respondPipelineMW{comp: modelComp}
	modelComp.AddMiddleware(rpMW)

	b.createPorts(modelComp, modelComp)

	if b.registrar != nil {
		b.registrar.RegisterComponent(modelComp)
	}

	return modelComp
}

func (b Builder) populateMemMapperSpec(spec *Spec) {
	if b.memPortMapper != nil {
		switch m := b.memPortMapper.(type) {
		case *mem.SinglePortMapper:
			spec.MemMapperKind = "single"
			spec.MemMapperPorts = []messaging.RemotePort{m.Port}
		case *mem.InterleavedAddressPortMapper:
			spec.MemMapperKind = "interleaved"
			spec.MemMapperPorts = m.LowModules
			spec.MemMapperInterleavingSize = m.InterleavingSize
		default:
			panic("unsupported memory port mapper type for spec conversion")
		}
		return
	}

	switch b.memPortMapperType {
	case "single":
		if len(b.memRemotePorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		spec.MemMapperKind = "single"
		spec.MemMapperPorts = b.memRemotePorts
	case "interleaved":
		if len(b.memRemotePorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		spec.MemMapperKind = "interleaved"
		spec.MemMapperPorts = b.memRemotePorts
		spec.MemMapperInterleavingSize = 1 << b.spec.Log2PageSize
	default:
		panic("invalid address mapper type: " + b.memPortMapperType)
	}
}

func (b Builder) populateTransMapperSpec(spec *Spec) {
	if b.translationPortMapper != nil {
		switch m := b.translationPortMapper.(type) {
		case *mem.SinglePortMapper:
			spec.TransMapperKind = "single"
			spec.TransMapperPorts = []messaging.RemotePort{m.Port}
		case *mem.InterleavedAddressPortMapper:
			spec.TransMapperKind = "interleaved"
			spec.TransMapperPorts = m.LowModules
			spec.TransMapperInterleavingSize = m.InterleavingSize
		default:
			panic("unsupported translation port mapper type for spec conversion")
		}
		return
	}

	switch b.translationPortMapperType {
	case "single":
		if len(b.translationRemotePorts) != 1 {
			panic("single translation mapper requires exactly 1 port")
		}
		spec.TransMapperKind = "single"
		spec.TransMapperPorts = b.translationRemotePorts
	case "interleaved":
		if len(b.translationRemotePorts) == 0 {
			panic("interleaved translation mapper requires at least 1 port")
		}
		spec.TransMapperKind = "interleaved"
		spec.TransMapperPorts = b.translationRemotePorts
		spec.TransMapperInterleavingSize = 1 << b.spec.Log2PageSize
	default:
		panic("invalid translation mapper type: " + b.translationPortMapperType)
	}
}

func (b Builder) createPorts(c messaging.Component, modelComp *modeling.Component[Spec, State, modeling.None]) {
	b.topPort.SetComponent(c)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(c)
	modelComp.AddPort("Bottom", b.bottomPort)

	b.translationPort.SetComponent(c)
	modelComp.AddPort("Translation", b.translationPort)

	b.ctrlPort.SetComponent(c)
	modelComp.AddPort("Control", b.ctrlPort)
}
