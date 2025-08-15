package tlb

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder can build TLBs
type Builder struct {
	engine         sim.Engine
	freq           sim.Freq
	numReqPerCycle int
	numSets        int
	numWays        int
	pageSize       uint64
	lowModule      sim.RemotePort
	numMSHREntry   int
	state          string
	latency        int
	addressMapper  mem.AddressToPortMapper
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
		state:          "enable",
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

// WithPageSize sets the page size that the TLB works with.
func (b Builder) WithPageSize(n uint64) Builder {
	b.pageSize = n
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
func (b Builder) WithLowModule(lowModule sim.RemotePort) Builder {
	b.lowModule = lowModule
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

func (b Builder) WithLatency(cycles int) Builder {
	b.latency = cycles
	return b
}

func (b Builder) WithAddressMapper(mapper mem.AddressToPortMapper) Builder {
	b.addressMapper = mapper
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

	b.createPorts(name, tlb)

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
