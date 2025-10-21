package tlb

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
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
	state             string
	latency           int
	addressMapper     mem.AddressToPortMapper
	addressMapperType string
	remotePorts       []sim.RemotePort
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
		state:          tlbStateEnable,
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
	b.addressMapper = &mem.SinglePortMapper{
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
	b.addressMapper = mapper
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

// Build creates a new TLB
func (b Builder) Build(name string) *Comp {
	tlb := &Comp{}
	tlb.TickingComponent =
		sim.NewTickingComponent(name, b.engine, b.freq, tlb)

	tlb.numSets = b.numSets
	tlb.numWays = b.numWays
	tlb.numReqPerCycle = b.numReqPerCycle
	tlb.pageSize = b.pageSize
	tlb.addressMapper = b.addressMapper
	tlb.mshr = newMSHR(b.numMSHREntry)
	tlb.state = b.state

	b.createPorts(name, tlb)
	b.createTranslationProviderMapper(tlb)

	tlb.reset()

	buf := sim.NewBuffer(name+".ResponsePipelineBuf", 16)
	tlb.responseBuffer = buf
	tlb.responsePipeline = pipelining.MakeBuilder().
		WithNumStage(b.latency).
		WithCyclePerStage(1).
		WithPipelineWidth(tlb.numReqPerCycle).
		WithPostPipelineBuffer(buf).
		Build(name + ".ResponsePipeline")

	ctrlMiddleware := &ctrlMiddleware{Comp: tlb}
	tlb.AddMiddleware(ctrlMiddleware)

	middleware := &tlbMiddleware{Comp: tlb}
	tlb.AddMiddleware(middleware)

	return tlb
}

func (b Builder) createTranslationProviderMapper(c *Comp) {
	if c.addressMapper != nil {
		return
	}

	switch b.addressMapperType {
	case "single":
		if len(b.remotePorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		c.addressMapper = &mem.SinglePortMapper{
			Port: b.remotePorts[0],
		}
	case "interleaved":
		if len(b.remotePorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		mapper := mem.NewInterleavedAddressPortMapper(1 << b.log2PageSize)
		mapper.LowModules = append(mapper.LowModules, b.remotePorts...)
		c.addressMapper = mapper
	default:
		panic("invalid address mapper type: " + b.addressMapperType)
	}
}

func (b Builder) createPorts(name string, c *Comp) {
	c.topPort = sim.NewPort(c,
		b.numReqPerCycle, b.numReqPerCycle,
		name+".TopPort")
	c.AddPort("Top", c.topPort)

	c.bottomPort = sim.NewPort(c,
		b.numReqPerCycle, b.numReqPerCycle,
		name+".BottomPort")
	c.AddPort("Bottom", c.bottomPort)

	c.controlPort = sim.NewPort(c, 1, 1,
		name+".ControlPort")
	c.AddPort("Control", c.controlPort)
}
