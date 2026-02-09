package mmuCache

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder can build mmuCache
type Builder struct {
	engine          sim.Engine
	freq            sim.Freq
	numReqPerCycle  int
	numLevels       int
	numBlocks       int
	pageSize        uint64
	segLength       int
	lowModule       sim.Port
	upModule        sim.Port
	numMSHREntry    int
	mshrEntryDepth  int
	isPrediction    bool
	bloomFilterSize int
	latencyPerLevel uint64
	log2PageSize    uint64

	maxInflightTransactions int
	inflightTransactions    int
	translationRequests     map[uint64]map[vm.PID]*vm.TranslationReq
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		freq:                    1 * sim.GHz,
		numReqPerCycle:          4,
		numLevels:               5,
		numBlocks:               1,
		pageSize:                4096,
		numMSHREntry:            64,
		mshrEntryDepth:          64,
		latencyPerLevel:         100,
		log2PageSize:            12,
		isPrediction:            false,
		bloomFilterSize:         64,
		maxInflightTransactions: 16,
		inflightTransactions:    0,
		translationRequests:     make(map[uint64]map[vm.PID]*vm.TranslationReq),
		segLength:               16,
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

func (b Builder) WithSegLength(length int) Builder {
	b.segLength = length
	return b
}

func (b Builder) WithMSHREntryDepth(depth int) Builder {
	b.mshrEntryDepth = depth
	return b
}

// WithEngine sets the engine that the mmuCache to use
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the mmuCache use
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumBlocks sets the number of blocks in a mmuCache.
func (b Builder) WithNumBlocks(n int) Builder {
	b.numBlocks = n
	return b
}

// WithPageSize sets the page size that the mmuCache works with.
func (b Builder) WithPageSize(n uint64) Builder {
	b.pageSize = n
	return b
}

// WithNumReqPerCycle sets the number of requests per cycle can be processed by
// a mmuCache
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithLowModule sets the port that can provide the address translation in case
// of mmuCache miss.
func (b Builder) WithLowModule(lowModule sim.Port) Builder {
	b.lowModule = lowModule
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

func (b Builder) WithPrediction() Builder {
	b.isPrediction = true
	return b
}

func (b Builder) WithBloomFilterSize(size int) Builder {
	b.bloomFilterSize = size
	return b
}

// Build creates a new mmuCache
func (b Builder) Build(name string) *Comp {
	if b.numBlocks <= 0 {
		panic("mmuCache.Builder: numBlocks must be > 0")
	}

	mmuCache := &Comp{}
	mmuCache.TickingComponent =
		sim.NewTickingComponent(name, b.engine, b.freq, mmuCache)

	mmuCache.numReqPerCycle = b.numReqPerCycle
	mmuCache.pageSize = b.pageSize
	mmuCache.numBlocks = b.numBlocks
	mmuCache.LowModule = b.lowModule
	mmuCache.numLevels = b.numLevels
	mmuCache.latencyPerLevel = b.latencyPerLevel
	mmuCache.UpModule = b.upModule
	mmuCache.log2PageSize = b.log2PageSize

	b.createPorts(name, mmuCache)

	mmuCache.AddMiddleware(&ctrlMiddleware{})
	mmuCache.AddMiddleware(&mmuCacheMiddleware{})

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
