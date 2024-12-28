package cache

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/mem/cache/internal/mshr"
	"github.com/sarchlab/akita/v4/mem/cache/internal/tagging"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// Builder can build caches.
type Builder struct {
	sim               simulation.Simulation
	freq              timing.Freq
	numReqPerCycle    int
	log2CacheLineSize int
	wayAssociativity  int
	cacheByteSize     uint64
	mshrCapacity      int
	cycleLatency      int
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
		mshrCapacity:      4,
		cycleLatency:      1,
		replaceStrategy:   "lru",
		writeStrategy:     "writeThrough",
	}
}

// WithSimulation sets the simulation of the builder.
func (b Builder) WithSimulation(sim simulation.Simulation) Builder {
	b.sim = sim
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

// WithMSHRCapacity sets the capacity of the MSHR of the builder.
func (b Builder) WithMSHRCapacity(mshrCapacity int) Builder {
	b.mshrCapacity = mshrCapacity
	return b
}

// WithCycleLatency sets the cycle latency of the builder.
func (b Builder) WithCycleLatency(cycleLatency int) Builder {
	b.cycleLatency = cycleLatency
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
		b.sim.GetEngine(),
		b.freq,
		comp,
	)

	b.initState(comp)
	b.addPorts(comp)
	b.addMiddleware(comp)

	return comp
}

func (b Builder) initState(comp *Comp) {
	blockSize := 1 << b.log2CacheLineSize
	numWays := b.wayAssociativity
	b.mustBeFullSets(b.cacheByteSize, blockSize, numWays)
	setSize := uint64(blockSize * numWays)
	numSets := int(b.cacheByteSize / setSize)

	comp.numReqPerCycle = b.numReqPerCycle
	comp.log2BlockSize = b.log2CacheLineSize
	comp.victimFinder = b.createVictimFinder()
	b.createInternalBuffers(comp)
	comp.mshr = mshr.NewMSHR(b.mshrCapacity)
	comp.storage = mem.NewStorage(b.cacheByteSize)
	comp.addressToDstTable = b.addressToDstTable
	comp.tags = tagging.NewTagArray(
		numSets,
		numWays,
		blockSize,
	)

	comp.state = &state{}
}

func (b Builder) createInternalBuffers(comp *Comp) {
	comp.storageTopDownBuf = queueing.BufferBuilder{}.
		WithSimulation(b.sim).
		WithCapacity(b.numReqPerCycle).
		Build("StorageTopDownBuf")
	comp.storageBottomUpBuf = queueing.BufferBuilder{}.
		WithSimulation(b.sim).
		WithCapacity(b.numReqPerCycle).
		Build("StorageBottomUpBuf")
	comp.bottomInteractionBuf = queueing.BufferBuilder{}.
		WithSimulation(b.sim).
		WithCapacity(b.numReqPerCycle).
		Build("BottomInteractionBuf")
	comp.storagePostPipelineBuf = queueing.BufferBuilder{}.
		WithSimulation(b.sim).
		WithCapacity(b.numReqPerCycle).
		Build("StoragePostPipelineBuf")

	comp.storagePipeline = queueing.PipelineBuilder{}.
		WithSimulation(b.sim).
		WithPipelineWidth(1).
		WithNumStage(b.cycleLatency).
		WithCyclePerStage(1).
		WithPostPipelineBuffer(comp.storagePostPipelineBuf).
		Build("StoragePipeline")
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

func (b Builder) addPorts(comp *Comp) {
	comp.topPort = modeling.PortBuilder{}.
		WithSimulation(b.sim).
		WithComponent(comp).
		WithIncomingBufCap(b.numReqPerCycle).
		WithOutgoingBufCap(b.numReqPerCycle).
		Build("Top")
	comp.AddPort("Top", comp.topPort)

	comp.bottomPort = modeling.PortBuilder{}.
		WithSimulation(b.sim).
		WithComponent(comp).
		WithIncomingBufCap(b.numReqPerCycle).
		WithOutgoingBufCap(b.numReqPerCycle).
		Build("Bottom")
	comp.AddPort("Bottom", comp.bottomPort)
}

func (b Builder) addMiddleware(comp *Comp) {
	comp.AddMiddleware(&storageMiddleware{Comp: comp})
	comp.AddMiddleware(&defaultReadStrategy{Comp: comp})
	comp.AddMiddleware(&bottomInteraction{Comp: comp})

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
