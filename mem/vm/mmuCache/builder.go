package mmuCache

import (
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder can build TLBs
type Builder struct {
	engine          sim.Engine
	freq            sim.Freq
	numReqPerCycle  int
	numLevels       int
	numBlocks       int
	pageSize        uint64
	lowModule       sim.Port
	upModule        sim.Port
	latencyPerLevel uint64
	log2PageSize    uint64
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		freq:            1 * sim.GHz,
		numReqPerCycle:  4,
		numLevels:       5,
		numBlocks:       1,
		pageSize:        4096,
		latencyPerLevel: 100,
		log2PageSize:    12,
	}
}

func (b Builder) WithLatencyPerLevel(latency uint64) Builder {
	b.latencyPerLevel = latency
	return b
}

func (b Builder) WithUpperModule(m sim.Port) Builder {
	b.upModule = m
	return b
}

// WithNumLevels sets the number of levels in the mmuCache
func (b Builder) WithNumLevels(n int) Builder {
	b.numLevels = n
	return b
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

// WithNumBlocks sets the number of blocks in a mmuCache.
func (b Builder) WithNumBlocks(n int) Builder {
	b.numBlocks = n
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
func (b Builder) WithLowModule(lowModule sim.Port) Builder {
	b.lowModule = lowModule
	return b
}

// Build creates a new TLB
func (b Builder) Build(name string) *Comp {
	mmuCache := &Comp{}
	mmuCache.TickingComponent =
		sim.NewTickingComponent(name, b.engine, b.freq, mmuCache)

	mmuCache.numReqPerCycle = b.numReqPerCycle
	mmuCache.pageSize = b.pageSize
	mmuCache.LowModule = b.lowModule
	mmuCache.numLevels = b.numLevels
	mmuCache.latencyPerLevel = b.latencyPerLevel
	mmuCache.UpModule = b.upModule
	mmuCache.log2PageSize = b.log2PageSize

	b.createPorts(name, mmuCache)

	mmuCache.reset()
	mmuCache.state = mmuCacheStateEnable

	return mmuCache
}

func (b Builder) createPorts(name string, mmuCache *Comp) {
	mmuCache.topPort = sim.NewPort(mmuCache, 4800, 4800,
		name+".TopPort")
	mmuCache.AddPort("Top", mmuCache.topPort)
	mmuCache.bottomPort = sim.NewPort(mmuCache, 4800, 4800,
		name+".BottomPort")
	mmuCache.AddPort("Bottom", mmuCache.bottomPort)

	mmuCache.controlPort = sim.NewPort(mmuCache, 1, 1,
		name+".ControlPort")
	mmuCache.AddPort("Control", mmuCache.controlPort)
}
