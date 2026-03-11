package writethrough

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// A Builder can build an writethrough cache
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
	addressToPortMapper   mem.AddressToPortMapper
	visTracer             tracing.Tracer

	addressMapperType string
	remotePorts       []sim.RemotePort
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

// WithAddressToPortMapper specifies how the cache units to create should find
// low level modules.
func (b Builder) WithAddressToPortMapper(
	addressToPortMapper mem.AddressToPortMapper,
) Builder {
	b.addressToPortMapper = addressToPortMapper
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

// Build returns a new cache component
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	b.assertAllRequiredInformationIsAvailable()

	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssociativity*blockSize))

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

	comp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	m := &middleware{
		comp: comp,
	}

	m.topPort = b.topPort
	m.topPort.SetComponent(comp)
	comp.AddPort("Top", m.topPort)
	m.bottomPort = b.bottomPort
	m.bottomPort.SetComponent(comp)
	comp.AddPort("Bottom", m.bottomPort)
	m.controlPort = b.controlPort
	m.controlPort.SetComponent(comp)
	comp.AddPort("Control", m.controlPort)

	m.dirBuf = queueing.NewBuffer(name+".DirectoryBuffer", b.numReqPerCycle)
	m.bankBufs = make([]queueing.Buffer, b.numBank)

	for i := 0; i < b.numBank; i++ {
		m.bankBufs[i] = queueing.NewBuffer(
			fmt.Sprintf("%s.Bank%d.Buffer", name, i),
			b.numReqPerCycle,
		)
	}

	// Initialize DirectoryState using free function instead of runtime Directory
	cache.DirectoryReset(&m.directoryState, numSets, b.wayAssociativity, blockSize)
	// MSHRState starts empty, no initialization needed
	m.storage = mem.NewStorage(b.totalByteSize)

	b.configureAddressMapper(m)

	b.buildStages(m)

	if b.visTracer != nil {
		tracing.CollectTrace(m, b.visTracer)
	}

	comp.AddMiddleware(m)

	return comp
}

func (b *Builder) buildStages(m *middleware) {
	m.coalesceStage = &coalescer{cache: m}
	b.buildDirStage(m)
	b.buildBankStages(m)
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}

	m.controlStage = &controlStage{
		ctrlPort:     m.controlPort,
		transactions: &m.transactions,
		cache:        m,
		bankStages:   m.bankStages,
		coalescer:    m.coalesceStage,
	}
}

func (b *Builder) buildDirStage(m *middleware) {
	buf := queueing.NewBuffer(
		m.comp.Name()+".DirectoryStage.PostPipelineBuffer",
		b.numReqPerCycle,
	)
	pipelineName := fmt.Sprintf("%s.Directory.Pipeline", m.comp.Name())
	pipeline := queueing.MakeBuilder().
		WithPipelineWidth(b.numReqPerCycle).
		WithNumStage(b.dirLatency).
		WithCyclePerStage(1).
		WithPostPipelineBuffer(buf).
		Build(pipelineName)
	m.directoryStage = &directory{
		cache:    m,
		buf:      buf,
		pipeline: pipeline,
	}
}

func (b *Builder) buildBankStages(m *middleware) {
	for i := 0; i < b.numBank; i++ {
		pipelineName := fmt.Sprintf("%s.Bank[%d].Pipeline", m.comp.Name(), i)
		postPipelineBuf := queueing.NewBuffer(
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", m.comp.Name(), i),
			b.numReqPerCycle,
		)
		pipeline := queueing.MakeBuilder().
			WithPipelineWidth(b.numReqPerCycle).
			WithNumStage(b.bankLatency).
			WithCyclePerStage(1).
			WithPostPipelineBuffer(postPipelineBuf).
			Build(pipelineName)
		bs := &bankStage{
			cache:           m,
			bankID:          i,
			numReqPerCycle:  b.numReqPerCycle,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
		m.bankStages = append(m.bankStages, bs)

		if b.visTracer != nil {
			tracing.CollectTrace(bs.pipeline, b.visTracer)
		}
	}
}

func (b *Builder) configureAddressMapper(m *middleware) {
	if b.addressToPortMapper != nil {
		m.addressToPortMapper = b.addressToPortMapper
		return
	}

	switch b.addressMapperType {
	case "single":
		if len(b.remotePorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		m.addressToPortMapper = &mem.SinglePortMapper{
			Port: b.remotePorts[0],
		}
	case "interleaved":
		if len(b.remotePorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		mapper := mem.NewInterleavedAddressPortMapper(4096)
		mapper.LowModules = append(mapper.LowModules, b.remotePorts...)
		m.addressToPortMapper = mapper
	default:
		panic("addressMapperType must be \"single\" or \"interleaved\"")
	}
}

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
