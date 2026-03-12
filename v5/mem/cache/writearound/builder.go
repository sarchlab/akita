package writearound

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// A Builder can build a writearound cache
type Builder struct {
	engine                sim.Engine
	freq                  sim.Freq
	log2BlockSize         uint64
	totalByteSize         uint64
	wayAssociativity      int
	numMSHREntry          int
	numBank               int
	dirLatency            int
	bankLatency           int
	numReqPerCycle        int
	maxNumConcurrentTrans int
	visTracer             tracing.Tracer

	addressMapperType string
	remotePorts       []sim.RemotePort
	interleavingSize  uint64
	legacyMapper      mem.AddressToPortMapper // set by WithAddressToPortMapper, read at Build time
	topPort           sim.Port
	bottomPort        sim.Port
	controlPort       sim.Port
}

// MakeBuilder creates a builder with default parameter setting
func MakeBuilder() Builder {
	return Builder{
		freq:                  1 * sim.GHz,
		log2BlockSize:         6,
		totalByteSize:         4 * mem.KB,
		wayAssociativity:      4,
		numMSHREntry:          4,
		numBank:               1,
		numReqPerCycle:        4,
		maxNumConcurrentTrans: 16,
		dirLatency:            2,
		bankLatency:           20,
		interleavingSize:      4096,
	}
}

// WithEngine sets the event driven simulation engine that the cache uses
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the cache works at
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithWayAssociativity sets the way associativity the builder builds.
func (b Builder) WithWayAssociativity(wayAssociativity int) Builder {
	b.wayAssociativity = wayAssociativity
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.numMSHREntry = num
	return b
}

// WithLog2BlockSize sets the number of bytes in a cache line as a power of 2
func (b Builder) WithLog2BlockSize(n uint64) Builder {
	b.log2BlockSize = n
	return b
}

// WithTotalByteSize sets the capacity of the cache unit
func (b Builder) WithTotalByteSize(byteSize uint64) Builder {
	b.totalByteSize = byteSize
	return b
}

// WithNumBanks sets the number of banks in each cache
func (b Builder) WithNumBanks(n int) Builder {
	b.numBank = n
	return b
}

// WithDirectoryLatency sets the number of cycles required to access the
// directory.
func (b Builder) WithDirectoryLatency(n int) Builder {
	b.dirLatency = n
	return b
}

// WithBankLatency sets the number of cycles needed to read to write a
// cacheline.
func (b Builder) WithBankLatency(n int) Builder {
	b.bankLatency = n
	return b
}

// WithMaxNumConcurrentTrans sets the maximum number of concurrent transactions
// that the cache can process.
func (b Builder) WithMaxNumConcurrentTrans(n int) Builder {
	b.maxNumConcurrentTrans = n
	return b
}

// WithNumReqsPerCycle sets the number of requests that the cache can process
// per cycle
func (b Builder) WithNumReqsPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithVisTracer sets the visualization tracer
func (b Builder) WithVisTracer(tracer tracing.Tracer) Builder {
	b.visTracer = tracer
	return b
}

// WithAddressMapperType sets the type of address mapper to use
func (b Builder) WithAddressMapperType(t string) Builder {
	b.addressMapperType = t
	return b
}

// WithRemotePorts sets the remote ports that the cache can use to send
// requests to other components.
func (b Builder) WithRemotePorts(ports ...sim.RemotePort) Builder {
	b.remotePorts = ports
	return b
}

// WithInterleavingSize sets the interleaving size for the address mapper.
func (b Builder) WithInterleavingSize(size uint64) Builder {
	b.interleavingSize = size
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

// WithAddressToPortMapper specifies how the cache units to create should find
// low level modules. This configures the address mapper using the legacy
// interface. Prefer WithAddressMapperType + WithRemotePorts for new code.
// The mapper is read at Build() time, so its fields can be set after this call.
func (b Builder) WithAddressToPortMapper(
	mapper mem.AddressToPortMapper,
) Builder {
	b.legacyMapper = mapper
	return b
}

// Build returns a new cache component
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.assertAllRequiredInformationIsAvailable()
	b.resolveLegacyMapper()

	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssociativity*blockSize))

	spec := b.buildSpec(numSets)

	initialState := State{
		BankBufIndices:             make([]bankBufState, b.numBank),
		BankPipelineStages:         make([]bankPipelineState, b.numBank),
		BankPostPipelineBufIndices: make([]bankPostBufState, b.numBank),
	}

	cache.DirectoryReset(
		&initialState.DirectoryState, numSets, b.wayAssociativity, blockSize)

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	comp.SetState(initialState)

	pmw := b.buildPipelineMW(comp)
	b.buildAdapters(pmw)
	b.buildStages(pmw)

	cmw := b.buildControlMW(comp, pmw)

	if b.visTracer != nil {
		tracing.CollectTrace(comp, b.visTracer)
	}

	comp.AddMiddleware(pmw) // index 0
	comp.AddMiddleware(cmw) // index 1

	return comp
}

func (b *Builder) buildSpec(numSets int) Spec {
	spec := Spec{
		NumReqPerCycle:        b.numReqPerCycle,
		Log2BlockSize:         b.log2BlockSize,
		BankLatency:           b.bankLatency,
		WayAssociativity:      b.wayAssociativity,
		MaxNumConcurrentTrans: b.maxNumConcurrentTrans,
		NumBanks:              b.numBank,
		NumMSHREntry:          b.numMSHREntry,
		NumSets:               numSets,
		TotalByteSize:         b.totalByteSize,
		DirLatency:            b.dirLatency,
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

func (b *Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State],
) *pipelineMW {
	m := &pipelineMW{comp: comp}

	m.topPort = b.topPort
	m.topPort.SetComponent(comp)
	comp.AddPort("Top", m.topPort)
	m.bottomPort = b.bottomPort
	m.bottomPort.SetComponent(comp)
	comp.AddPort("Bottom", m.bottomPort)

	m.storage = mem.NewStorage(b.totalByteSize)

	return m
}

func (b *Builder) buildControlMW(
	comp *modeling.Component[Spec, State],
	pmw *pipelineMW,
) *controlMW {
	controlPort := b.controlPort
	controlPort.SetComponent(comp)
	comp.AddPort("Control", controlPort)

	cs := &controlStage{
		ctrlPort:     controlPort,
		transactions: &pmw.transactions,
		pipeline:     pmw,
		bankStages:   pmw.bankStages,
		coalescer:    pmw.coalesceStage,
	}

	cmw := &controlMW{
		comp:         comp,
		controlStage: cs,
	}

	return cmw
}

func (b *Builder) buildAdapters(m *pipelineMW) {
	next := m.comp.GetNextState()

	// Dir buf adapter (read/write pointers set by updateAdapterPointers each tick)
	m.dirBufAdapter = &stateTransBuffer{
		name:       m.comp.Name() + ".DirectoryBuffer",
		readItems:  &next.DirBufIndices,
		writeItems: &next.DirBufIndices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	// Bank buf adapters
	m.bankBufAdapters = make([]*stateTransBuffer, b.numBank)
	for i := 0; i < b.numBank; i++ {
		m.bankBufAdapters[i] = &stateTransBuffer{
			name:       fmt.Sprintf("%s.Bank%d.Buffer", m.comp.Name(), i),
			readItems:  &next.BankBufIndices[i].Indices,
			writeItems: &next.BankBufIndices[i].Indices,
			capacity:   b.numReqPerCycle,
			mw:         m,
		}
	}

	// Dir post pipeline buf adapter
	m.dirPostBufAdapter = &stateDirPostBufAdapter{
		name:       m.comp.Name() + ".DirectoryStage.PostPipelineBuffer",
		readItems:  &next.DirPostPipelineBufIndices,
		writeItems: &next.DirPostPipelineBufIndices,
		capacity:   b.numReqPerCycle,
		mw:         m,
	}

	// Bank post pipeline buf adapters
	m.bankPostBufAdapters = make([]*stateBankPostBufAdapter, b.numBank)
	for i := 0; i < b.numBank; i++ {
		m.bankPostBufAdapters[i] = &stateBankPostBufAdapter{
			name: fmt.Sprintf(
				"%s.Bank[%d].PostPipelineBuffer", m.comp.Name(), i),
			readItems:  &next.BankPostPipelineBufIndices[i].Indices,
			writeItems: &next.BankPostPipelineBufIndices[i].Indices,
			capacity:   b.numReqPerCycle,
			mw:         m,
		}
	}
}

func (b *Builder) buildStages(m *pipelineMW) {
	m.coalesceStage = &coalescer{cache: m}
	m.directoryStage = &directory{cache: m}
	b.buildBankStages(m)
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}
}

func (b *Builder) buildBankStages(m *pipelineMW) {
	for i := 0; i < b.numBank; i++ {
		bs := &bankStage{
			cache:          m,
			bankID:         i,
			numReqPerCycle: b.numReqPerCycle,
		}
		m.bankStages = append(m.bankStages, bs)
	}
}

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

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
