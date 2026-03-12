package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// resolveLegacyMapper converts a legacy AddressToPortMapper set via
// WithAddressToPortMapper into the builder's addressMapperType/remotePorts/
// interleavingSize fields. This allows Build() to always populate Spec from
// the builder, matching the writearound pattern.
func (b *Builder) resolveLegacyMapper() {
	if b.legacyMapper == nil {
		return
	}

	switch m := b.legacyMapper.(type) {
	case *mem.SinglePortMapper:
		b.addressMapperType = "single"
		b.remotePorts = []sim.RemotePort{m.Port}
	case *mem.InterleavedAddressPortMapper:
		b.addressMapperType = "interleaved"
		b.remotePorts = m.LowModules
		b.interleavingSize = m.InterleavingSize
	default:
		panic(fmt.Sprintf("unsupported address mapper type: %T", b.legacyMapper))
	}

	b.legacyMapper = nil
}

// A Builder can build writeback caches
type Builder struct {
	engine           sim.Engine
	freq             sim.Freq
	legacyMapper     mem.AddressToPortMapper
	wayAssociativity int
	log2BlockSize    uint64

	byteSize            uint64
	numMSHREntry        int
	numReqPerCycle      int
	writeBufferCapacity int
	maxInflightFetch    int
	maxInflightEviction int

	dirLatency  int
	bankLatency int

	addressMapperType string
	remotePorts       []sim.RemotePort
	interleavingSize  uint64

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
		interleavingSize:    4096,
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
// The mapper is read lazily at Tick time, so its fields can be set after Build.
func (b Builder) WithAddressToPortMapper(f mem.AddressToPortMapper) Builder {
	b.legacyMapper = f
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

// WithWriteBufferSize sets the number of cache lines that can reside in the
// write buffer.
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

// WithBankLatency sets the number of cycles required to process each cache
// read/write operation.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

// WithAddressMapperType sets the address mapper type.
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

// WithRemotePorts sets the remote ports for address mapping.
func (b Builder) WithRemotePorts(ports ...sim.RemotePort) Builder {
	b.remotePorts = ports
	return b
}

// WithInterleavingSize sets the interleaving size for the address mapper.
func (b Builder) WithInterleavingSize(size uint64) Builder {
	b.interleavingSize = size
	return b
}

// Build creates a usable writeback cache.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.resolveLegacyMapper()

	blockSize := 1 << b.log2BlockSize
	numSets := int(b.byteSize / uint64(b.wayAssociativity*blockSize))

	spec := b.buildSpec(numSets)

	laneWidth := b.numReqPerCycle
	if laneWidth == 1 {
		laneWidth = 2
	}

	initialState := State{
		DirToBankBufIndices:             make([]bankBufState, 1),
		WriteBufferToBankBufIndices:     make([]bankBufState, 1),
		BankPipelineStages:              make([]bankPipelineState, 1),
		BankPostPipelineBufIndices:      make([]bankPostBufState, 1),
		BankInflightTransCounts:         make([]int, 1),
		BankDownwardInflightTransCounts: make([]int, 1),
	}

	// Initialize directory state
	cache.DirectoryReset(
		&initialState.DirectoryState, numSets, b.wayAssociativity, blockSize)

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	comp.SetState(initialState)

	m := &middleware{
		comp:         comp,
		state:        cacheStateRunning,
		evictingList: make(map[uint64]bool),
	}

	b.createPorts(m, comp)
	m.storage = mem.NewStorage(b.byteSize)

	b.buildAdapters(m, laneWidth)
	b.createInternalStages(m, laneWidth)

	comp.AddMiddleware(m)

	return comp
}

func (b *Builder) buildSpec(numSets int) Spec {
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

	if b.addressMapperType != "" {
		remotePortNames := make([]string, len(b.remotePorts))
		for i, rp := range b.remotePorts {
			remotePortNames[i] = string(rp)
		}
		spec.AddressMapperType = b.addressMapperType
		spec.RemotePortNames = remotePortNames
		spec.InterleavingSize = b.interleavingSize
	}

	return spec
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

func (b *Builder) buildAdapters(m *middleware, laneWidth int) {
	next := m.comp.GetNextState()

	b.buildTransBufferAdapters(m, next)
	b.buildPostBufAdapters(m, next, laneWidth)
}

func (b *Builder) buildTransBufferAdapters(m *middleware, next *State) {
	m.dirStageBuffer = &stateTransBuffer{
		name:       m.comp.Name() + ".DirStageBuffer",
		readItems:  &next.DirStageBufIndices,
		writeItems: &next.DirStageBufIndices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	m.dirToBankBuffers = make([]*stateTransBuffer, 1)
	m.dirToBankBuffers[0] = &stateTransBuffer{
		name:       m.comp.Name() + ".DirToBankBuffer",
		readItems:  &next.DirToBankBufIndices[0].Indices,
		writeItems: &next.DirToBankBufIndices[0].Indices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	m.writeBufferToBankBuffers = make([]*stateTransBuffer, 1)
	m.writeBufferToBankBuffers[0] = &stateTransBuffer{
		name:       m.comp.Name() + ".WriteBufferToBankBuffer",
		readItems:  &next.WriteBufferToBankBufIndices[0].Indices,
		writeItems: &next.WriteBufferToBankBufIndices[0].Indices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	m.mshrStageBuffer = &stateTransBuffer{
		name:       m.comp.Name() + ".MSHRStageBuffer",
		readItems:  &next.MSHRStageBufEntries,
		writeItems: &next.MSHRStageBufEntries,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	m.writeBufferBuffer = &stateTransBuffer{
		name:       m.comp.Name() + ".WriteBufferBuffer",
		readItems:  &next.WriteBufferBufIndices,
		writeItems: &next.WriteBufferBufIndices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}
}

func (b *Builder) buildPostBufAdapters(
	m *middleware, next *State, laneWidth int,
) {
	m.dirPostBufAdapter = &stateDirPostBufAdapter{
		name:       m.comp.Name() + ".DirectoryStage.PostPipelineBuffer",
		readItems:  &next.DirPostPipelineBufIndices,
		writeItems: &next.DirPostPipelineBufIndices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	m.bankPostBufAdapters = make([]*stateBankPostBufAdapter, 1)
	m.bankPostBufAdapters[0] = &stateBankPostBufAdapter{
		name: fmt.Sprintf(
			"%s.Bank.PostPipelineBuffer", m.comp.Name()),
		readItems:  &next.BankPostPipelineBufIndices[0].Indices,
		writeItems: &next.BankPostPipelineBufIndices[0].Indices,
		capacity:   laneWidth,
		mw:         m,
	}
}

func (b *Builder) createInternalStages(m *middleware, laneWidth int) {
	m.topParser = &topParser{cache: m}

	m.dirStage = &directoryStage{
		cache: m,
	}

	m.bankStages = make([]*bankStage, 1)
	m.bankStages[0] = &bankStage{
		cache:         m,
		bankID:        0,
		pipelineWidth: laneWidth,
	}

	m.mshrStage = &mshrStage{cache: m}
	m.flusher = &flusher{cache: m}
	m.writeBuffer = &writeBufferStage{
		cache:               m,
		writeBufferCapacity: b.writeBufferCapacity,
		maxInflightFetch:    b.maxInflightFetch,
		maxInflightEviction: b.maxInflightEviction,
	}
}
