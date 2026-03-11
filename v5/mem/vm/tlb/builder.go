package tlb

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder can build TLBs
type Builder struct {
	engine            sim.Engine
	freq              sim.Freq
	numReqPerCycle    int
	numSets           int
	numWays           int
	log2PageSize      uint64
	pageSize          uint64
	numMSHREntry      int
	latency           int
	addressMapperType string
	remotePorts       []sim.RemotePort
	topPort           sim.Port
	bottomPort        sim.Port
	controlPort       sim.Port

	// Legacy support for WithTranslationProviderMapper
	legacyMapper mem.AddressToPortMapper
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		freq:           1 * sim.GHz,
		numReqPerCycle: 4,
		numSets:        1,
		numWays:        32,
		pageSize:       4096,
		numMSHREntry:   4,
		latency:        4,
	}
}

// WithEngine sets the engine that the TLBs to use
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the TLBs use
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumSets sets the number of sets in a TLB. Use 1 for fully associated
// TLBs.
func (b Builder) WithNumSets(n int) Builder {
	b.numSets = n
	return b
}

// WithNumWays sets the number of ways in a TLB. Set this field to the number
// of TLB entries for all the functions.
func (b Builder) WithNumWays(n int) Builder {
	b.numWays = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	b.pageSize = 1 << n
	return b
}

// WithPageSize sets the page size that the TLB works with.
//
// Deprecated: Use `WithLog2PageSize` instead.
func (b Builder) WithPageSize(n uint64) Builder {
	// Check if n is a power of 2 by counting the number of 1s in binary
	if n == 0 || (n&(n-1)) != 0 {
		panic("page size must be a power of 2")
	}

	log2 := 0
	temp := n

	for temp > 0 {
		temp >>= 1
		log2++
	}

	b.log2PageSize = uint64(log2 - 1) // Subtract 1 because we count one extra iteration
	b.pageSize = 1 << b.log2PageSize

	return b
}

// WithNumReqPerCycle sets the number of requests per cycle can be processed by
// a TLB
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithLowModule sets the port that can provide the address translation in case
// of tlb miss.
//
// Deprecated: Use `WithTranslationProviderMapper` instead.
func (b Builder) WithLowModule(lowModule sim.RemotePort) Builder {
	b.legacyMapper = &mem.SinglePortMapper{
		Port: lowModule,
	}
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

// WithLatency sets the latency of the TLB lookup. The latency is counted in
// both hit and miss cases.
func (b Builder) WithLatency(cycles int) Builder {
	b.latency = cycles
	return b
}

// WithTranslationProviderMapper sets the mapper that can find the remote port
// that can provide the translation service according to the virtual address.
func (b Builder) WithTranslationProviderMapper(
	mapper mem.AddressToPortMapper,
) Builder {
	b.legacyMapper = mapper
	return b
}

// WithTranslationProviderMapperType sets the type of the translation provider
// mapper. The mapper can find the remote port that can provide the translation
// service according to the virtual address. The type can be "single" or
// "interleaved".
func (b Builder) WithTranslationProviderMapperType(t string) Builder {
	b.addressMapperType = t
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
func (b Builder) WithTranslationProviders(ports ...sim.RemotePort) Builder {
	b.remotePorts = ports
	return b
}

// WithTopPort sets the top port for the TLB
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port for the TLB
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

// WithControlPort sets the control port for the TLB
func (b Builder) WithControlPort(port sim.Port) Builder {
	b.controlPort = port
	return b
}

// Build creates a new TLB
func (b Builder) Build(name string) *Comp {
	addrMapperKind, addrMapperPorts, addrMapperInterleavingSize := b.resolveAddressMapper()

	spec := Spec{
		NumSets:                    b.numSets,
		NumWays:                    b.numWays,
		PageSize:                   b.pageSize,
		NumReqPerCycle:             b.numReqPerCycle,
		MSHRSize:                   b.numMSHREntry,
		Latency:                    b.latency,
		PipelineWidth:              b.numReqPerCycle,
		AddrMapperKind:             addrMapperKind,
		AddrMapperPorts:            addrMapperPorts,
		AddrMapperInterleavingSize: addrMapperInterleavingSize,
	}

	initialState := State{
		TLBState:          tlbStateEnable,
		Sets:              initSets(b.numSets, b.numWays),
		PipelineNumStages: b.latency,
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	c := &Comp{
		Component: modelComp,
	}

	b.topPort.SetComponent(c)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(c)
	modelComp.AddPort("Bottom", b.bottomPort)

	b.controlPort.SetComponent(c)
	modelComp.AddPort("Control", b.controlPort)

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	tlbMW := &tlbMiddleware{comp: modelComp}
	modelComp.AddMiddleware(tlbMW)

	return c
}

func (b Builder) resolveAddressMapper() (string, []sim.RemotePort, uint64) {
	if b.legacyMapper != nil {
		// Convert legacy mapper to spec fields
		switch m := b.legacyMapper.(type) {
		case *mem.SinglePortMapper:
			return "single", []sim.RemotePort{m.Port}, 0
		case *mem.InterleavedAddressPortMapper:
			return "interleaved", m.LowModules, m.InterleavingSize
		default:
			// For any unknown mapper type, try to use it as single port
			// Fall through to the addressMapperType logic
		}
	}

	interleavingSize := b.pageSize
	if interleavingSize == 0 {
		interleavingSize = 4096
	}

	if b.addressMapperType != "" {
		switch b.addressMapperType {
		case "single":
			if len(b.remotePorts) != 1 {
				panic("single address mapper requires exactly 1 port")
			}
			return "single", b.remotePorts, 0
		case "interleaved":
			if len(b.remotePorts) == 0 {
				panic("interleaved address mapper requires at least 1 port")
			}
			return "interleaved", b.remotePorts, interleavingSize
		default:
			panic("invalid address mapper type: " + b.addressMapperType)
		}
	}

	panic("no address mapper configured")
}
