package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder can build writeback caches
type Builder struct {
	engine              sim.Engine
	freq                sim.Freq
	addressToPortMapper mem.AddressToPortMapper
	wayAssociativity    int
	log2BlockSize       uint64

	interleaving          bool
	numInterleavingBlock  int
	interleavingUnitCount int
	interleavingUnitIndex int

	byteSize            uint64
	numMSHREntry        int
	numReqPerCycle      int
	writeBufferCapacity int
	maxInflightFetch    int
	maxInflightEviction int

	dirLatency  int
	bankLatency int

	addressMapperType string

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{
		freq:                1 * sim.GHz,
		wayAssociativity:    4,
		log2BlockSize:       6,
		byteSize:            512 * mem.KB,
		numMSHREntry:        16,
		numReqPerCycle:      1,
		writeBufferCapacity: 1024,
		maxInflightFetch:    128,
		maxInflightEviction: 128,
		bankLatency:         10,
	}
}

// WithEngine sets the engine to be used by the caches.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency to be used by the caches.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithWayAssociativity sets the way associativity.
func (b Builder) WithWayAssociativity(n int) Builder {
	b.wayAssociativity = n
	return b
}

// WithLog2BlockSize sets the cache line size as the power of 2.
func (b Builder) WithLog2BlockSize(n uint64) Builder {
	b.log2BlockSize = n
	return b
}

// WithNumMSHREntry sets the number of MSHR entries.
func (b Builder) WithNumMSHREntry(n int) Builder {
	b.numMSHREntry = n
	return b
}

// WithAddressToPortMapper sets the AddressToPortMapper to be used.
func (b Builder) WithAddressToPortMapper(f mem.AddressToPortMapper) Builder {
	b.addressToPortMapper = f
	return b
}

// WithNumReqPerCycle sets the number of requests that can be processed by the
// cache in each cycle.
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithByteSize set the size of the cache.
func (b Builder) WithByteSize(byteSize uint64) Builder {
	b.byteSize = byteSize
	return b
}

// WithInterleaving sets the size that the cache is interleaved.
func (b Builder) WithInterleaving(
	numBlock, unitCount, unitIndex int,
) Builder {
	b.interleaving = true
	b.numInterleavingBlock = numBlock
	b.interleavingUnitCount = unitCount
	b.interleavingUnitIndex = unitIndex

	return b
}

// WithWriteBufferSize sets the number of cach lines that can reside in the
// writebuffer.
func (b Builder) WithWriteBufferSize(n int) Builder {
	b.writeBufferCapacity = n
	return b
}

// WithMaxInflightFetch sets the number of concurrent fetch that the write-back
// cache can issue at the same time.
func (b Builder) WithMaxInflightFetch(n int) Builder {
	b.maxInflightFetch = n
	return b
}

// WithMaxInflightEviction sets the number of concurrent eviction that the
// write buffer can write to a low-level module.
func (b Builder) WithMaxInflightEviction(n int) Builder {
	b.maxInflightEviction = n
	return b
}

// WithDirectoryLatency sets the number of cycles required to access the
// directory.
func (b Builder) WithDirectoryLatency(n int) Builder {
	b.dirLatency = n
	return b
}

// WithBankLatency sets the number of cycles required to process each can
// read/write operation.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

func (b Builder) WithAddressMapperType(t string) Builder {
	b.addressMapperType = t
	return b
}

// WithTopPort sets the top port for the cache
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port for the cache
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

// WithControlPort sets the control port for the cache
func (b Builder) WithControlPort(port sim.Port) Builder {
	b.controlPort = port
	return b
}

func (b Builder) WithRemotePorts(ports ...sim.RemotePort) Builder {
	if b.addressMapperType == "single" {
		if len(ports) != 1 {
			panic("single address mapper requires exactly 1 port")
		}

		b.addressToPortMapper = &mem.SinglePortMapper{Port: ports[0]}
	} else if b.addressMapperType == "interleaved" {
		finder := mem.NewInterleavedAddressPortMapper(256)
		finder.LowModules = append(finder.LowModules, ports...)
		b.addressToPortMapper = finder
	} else {
		panic("unknown address mapper type")
	}

	return b
}

// Build creates a usable writeback cache.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	blockSize := 1 << b.log2BlockSize
	numSets := int(b.byteSize / uint64(b.wayAssociativity*blockSize))

	spec := Spec{
		NumReqPerCycle:      b.numReqPerCycle,
		Log2BlockSize:       b.log2BlockSize,
		BankLatency:         b.bankLatency,
		WayAssociativity:    b.wayAssociativity,
		NumBanks:            1,
		NumSets:             numSets,
		NumMSHREntry:        b.numMSHREntry,
		TotalByteSize:       b.byteSize,
		DirLatency:          b.dirLatency,
		WriteBufferCapacity: b.writeBufferCapacity,
		MaxInflightFetch:    b.maxInflightFetch,
		MaxInflightEviction: b.maxInflightEviction,
	}

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	m := &middleware{
		comp:             comp,
		log2BlockSize:    b.log2BlockSize,
		numReqPerCycle:   b.numReqPerCycle,
		wayAssociativity: b.wayAssociativity,
		numMSHREntry:     b.numMSHREntry,
		numSets:          numSets,
		blockSize:        blockSize,
		state:            cacheStateRunning,
		evictingList:     make(map[uint64]bool),
	}

	b.createPorts(m, comp)
	b.configureCache(m)
	b.createInternalStages(m)
	b.createInternalBuffers(m)

	comp.AddMiddleware(m)

	return comp
}

func (b *Builder) configureCache(m *middleware) {
	// Initialize DirectoryState using free function
	cache.DirectoryReset(
		&m.directoryState,
		m.numSets,
		b.wayAssociativity,
		m.blockSize,
	)

	// MSHRState starts empty, no initialization needed
	m.storage = mem.NewStorage(b.byteSize)

	if b.addressToPortMapper == nil {
		panic(
			"addressToPortMapper is nil. " +
				"WithRemotePorts or WithAddressMapperType not set",
		)
	}

	m.addressToPortMapper = b.addressToPortMapper
}

func (b *Builder) createPorts(m *middleware, comp *modeling.Component[Spec, State]) {
	m.topPort = b.topPort
	m.topPort.SetComponent(comp)
	comp.AddPort("Top", m.topPort)

	m.bottomPort = b.bottomPort
	m.bottomPort.SetComponent(comp)
	comp.AddPort("Bottom", m.bottomPort)

	m.controlPort = b.controlPort
	m.controlPort.SetComponent(comp)
	comp.AddPort("Control", m.controlPort)
}

func (b *Builder) createInternalStages(m *middleware) {
	m.topParser = &topParser{cache: m}
	b.buildDirectoryStage(m)
	b.buildBankStages(m)
	m.mshrStage = &mshrStage{cache: m}
	m.flusher = &flusher{cache: m}
	m.writeBuffer = &writeBufferStage{
		cache:               m,
		writeBufferCapacity: b.writeBufferCapacity,
		maxInflightFetch:    b.maxInflightFetch,
		maxInflightEviction: b.maxInflightEviction,
	}
}

func (b *Builder) buildDirectoryStage(m *middleware) {
	buf := queueing.NewBuffer(
		m.comp.Name()+".DirectoryStageBuffer",
		b.numReqPerCycle,
	)
	pipeline := queueing.
		MakeBuilder().
		WithCyclePerStage(1).
		WithNumStage(b.dirLatency).
		WithPipelineWidth(b.numReqPerCycle).
		WithPostPipelineBuffer(buf).
		Build(m.comp.Name() + ".BankPipeline")
	m.dirStage = &directoryStage{
		cache:    m,
		pipeline: pipeline,
		buf:      buf,
	}
}

func (b *Builder) buildBankStages(m *middleware) {
	m.bankStages = make([]*bankStage, 1)

	laneWidth := b.numReqPerCycle
	if laneWidth == 1 {
		laneWidth = 2
	}

	buf := queueing.NewBuffer(
		fmt.Sprintf("%s.Bank.PostPipelineBuffer", m.comp.Name()),
		laneWidth,
	)
	pipeline := queueing.
		MakeBuilder().
		WithCyclePerStage(1).
		WithNumStage(b.bankLatency).
		WithPipelineWidth(laneWidth).
		WithPostPipelineBuffer(buf).
		Build(fmt.Sprintf("%s.Bank.Pipeline", m.comp.Name()))
	m.bankStages[0] = &bankStage{
		cache:           m,
		bankID:          0,
		pipeline:        pipeline,
		postPipelineBuf: buf,
		pipelineWidth:   laneWidth,
	}
}

func (b *Builder) createInternalBuffers(m *middleware) {
	m.dirStageBuffer = queueing.NewBuffer(
		m.comp.Name()+".DirStageBuffer",
		m.numReqPerCycle,
	)
	m.dirToBankBuffers = make([]queueing.Buffer, 1)
	m.dirToBankBuffers[0] = queueing.NewBuffer(
		m.comp.Name()+".DirToBankBuffer",
		m.numReqPerCycle,
	)
	m.writeBufferToBankBuffers = make([]queueing.Buffer, 1)
	m.writeBufferToBankBuffers[0] = queueing.NewBuffer(
		m.comp.Name()+".WriteBufferToBankBuffer",
		m.numReqPerCycle,
	)
	m.mshrStageBuffer = queueing.NewBuffer(
		m.comp.Name()+".MSHRStageBuffer",
		m.numReqPerCycle,
	)
	m.writeBufferBuffer = queueing.NewBuffer(
		m.comp.Name()+".WriteBufferBuffer",
		m.numReqPerCycle,
	)
}
