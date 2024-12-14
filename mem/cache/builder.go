package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// Builder can build caches.
type Builder struct {
	engine timing.Engine
	freq   timing.Freq

	numReqPerCycle    int
	log2CacheLineSize int
	wayAssociativity  int
	cacheByteSize     uint64
	replaceStrategy   string
	writeStrategy     string
	addressToDstTable mem.AddressToPortMapper
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		freq:              1 * timing.GHz,
		numReqPerCycle:    1,
		log2CacheLineSize: 6,
		wayAssociativity:  4,
		cacheByteSize:     16 * mem.KB,
		replaceStrategy:   "lru",
		writeStrategy:     "writeThrough",
	}
}

// WithEngine sets the engine of the builder.
func (b Builder) WithEngine(engine timing.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the builder.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumReqPerCycle sets the number of requests per cycle of the builder.
func (b Builder) WithNumReqPerCycle(numReqPerCycle int) Builder {
	b.numReqPerCycle = numReqPerCycle
	return b
}

// WithLog2CacheLineSize sets the log2 of the cache line size of the builder.
func (b Builder) WithLog2CacheLineSize(log2CacheLineSize int) Builder {
	b.log2CacheLineSize = log2CacheLineSize
	return b
}

// WithWayAssociativity sets the way associativity of the builder.
func (b Builder) WithWayAssociativity(wayAssociativity int) Builder {
	b.wayAssociativity = wayAssociativity
	return b
}

// WithWriteStrategy sets the write strategy of the builder.
func (b Builder) WithWriteStrategy(writeStrategy string) Builder {
	b.writeStrategy = writeStrategy
	return b
}

// WithAddressToDstTable sets the address to dst table of the builder.
func (b Builder) WithAddressToDstTable(
	addressToDstTable mem.AddressToPortMapper,
) Builder {
	b.addressToDstTable = addressToDstTable
	return b
}

// Build builds a cache.
func (b Builder) Build(name string) *Comp {
	comp := new(Comp)
	comp.TickingComponent = modeling.NewTickingComponent(
		name,
		b.engine,
		b.freq,
		comp,
	)

	b.initState(comp)
	b.addPorts(comp, name)
	b.addMiddleware(comp)

	return comp
}

func (b Builder) initState(comp *Comp) {
	blockSize := 1 << b.log2CacheLineSize
	numWays := b.wayAssociativity
	b.mustBeFullSets(b.cacheByteSize, blockSize, numWays)
	setSize := uint64(blockSize * numWays)
	numSets := int(b.cacheByteSize / setSize)

	victimFinder := b.createVictimFinder()
	tags := tagging.NewTags(numSets, numWays, blockSize, victimFinder)

	comp.state = state{
		NumReqPerCycle:    b.numReqPerCycle,
		Log2BlockSize:     b.log2CacheLineSize,
		MSHR:              mshr.NewMSHR(b.wayAssociativity),
		Tags:              tags,
		Storage:           mem.NewStorage(b.cacheByteSize),
		AddressToDstTable: b.addressToDstTable,
		EvictQueue: queueing.NewBuffer(
			comp.Name()+".EvictQueue",
			b.numReqPerCycle,
		),
	}
}

func (b Builder) createVictimFinder() tagging.VictimFinder {
	var victimFinder tagging.VictimFinder

	switch b.replaceStrategy {
	case "lru":
		victimFinder = tagging.NewLRUVictimFinder()
	default:
		panic("unknown replace strategy: " + b.replaceStrategy)
	}

	return victimFinder
}

func (b Builder) addPorts(comp *Comp, name string) {
	comp.topPort = modeling.NewPort(
		comp,
		b.numReqPerCycle,
		b.log2CacheLineSize,
		name+".Top",
	)
	comp.bottomPort = modeling.NewPort(
		comp,
		b.numReqPerCycle,
		b.log2CacheLineSize,
		name+".Bottom",
	)

	comp.AddPort("Top", comp.topPort)
	comp.AddPort("Bottom", comp.bottomPort)
}

func (b Builder) addMiddleware(comp *Comp) {
	comp.AddMiddleware(&storageMiddleware{Comp: comp})
	comp.AddMiddleware(&defaultReadStrategy{Comp: comp})

	switch b.writeStrategy {
	case "writeThrough":
		comp.AddMiddleware(&writeThroughStrategy{Comp: comp})
	case "writeBack":
		comp.AddMiddleware(&writeBackStrategy{Comp: comp})
	case "readOnly":
		// Do nothing.
	default:
		panic("unknown write strategy: " + b.writeStrategy)
	}
}

func (b Builder) mustBeFullSets(cacheByteSize uint64, blockSize, numWays int) {
	setSize := uint64(blockSize * numWays)
	if cacheByteSize%setSize != 0 {
		panic("cache must have a integer number of sets")
	}
}
