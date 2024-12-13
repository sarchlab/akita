package tlb

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Builder can build TLBs
type Builder struct {
	engine         timing.Engine
	freq           timing.Freq
	numReqPerCycle int
	numSets        int
	numWays        int
	pageSize       uint64
	lowModule      modeling.RemotePort
	numMSHREntry   int
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		freq:           1 * timing.GHz,
		numReqPerCycle: 4,
		numSets:        1,
		numWays:        32,
		pageSize:       4096,
		numMSHREntry:   4,
	}
}

// WithEngine sets the engine that the TLBs to use
func (b Builder) WithEngine(engine timing.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the TLBs use
func (b Builder) WithFreq(freq timing.Freq) Builder {
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
func (b Builder) WithLowModule(lowModule modeling.RemotePort) Builder {
	b.lowModule = lowModule
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

// Build creates a new TLB
func (b Builder) Build(name string) *Comp {
	tlb := &Comp{}
	tlb.TickingComponent =
		modeling.NewTickingComponent(name, b.engine, b.freq, tlb)

	tlb.numSets = b.numSets
	tlb.numWays = b.numWays
	tlb.numReqPerCycle = b.numReqPerCycle
	tlb.pageSize = b.pageSize
	tlb.LowModule = b.lowModule
	tlb.mshr = newMSHR(b.numMSHREntry)

	b.createPorts(name, tlb)

	tlb.reset()

	middleware := &middleware{Comp: tlb}
	tlb.AddMiddleware(middleware)

	return tlb
}

func (b Builder) createPorts(name string, c *Comp) {
	c.topPort = modeling.NewPort(c,
		b.numReqPerCycle, b.numReqPerCycle,
		name+".TopPort")
	c.AddPort("Top", c.topPort)

	c.bottomPort = modeling.NewPort(c,
		b.numReqPerCycle, b.numReqPerCycle,
		name+".BottomPort")
	c.AddPort("Bottom", c.bottomPort)

	c.controlPort = modeling.NewPort(c, 1, 1,
		name+".ControlPort")
	c.AddPort("Control", c.controlPort)
}
