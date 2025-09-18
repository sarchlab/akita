package writearound

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// A Builder can build an writearound cache
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

// Build returns a new cache unit
func (b Builder) Build(name string) *Comp {
	b.assertAllRequiredInformationIsAvailable()

	c := &Comp{
		log2BlockSize:  b.log2BlockSize,
		numReqPerCycle: b.numReqPerCycle,
	}
	c.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, c)

	c.topPort = sim.NewPort(c, b.numReqPerCycle, b.numReqPerCycle,
		name+".TopPort")
	c.AddPort("Top", c.topPort)
	c.bottomPort = sim.NewPort(c, b.numReqPerCycle, b.numReqPerCycle,
		name+".BottomPort")
	c.AddPort("Bottom", c.bottomPort)
	c.controlPort = sim.NewPort(c, b.numReqPerCycle, b.numReqPerCycle,
		name+".ControlPort")
	c.AddPort("Control", c.controlPort)

	c.dirBuf = sim.NewBuffer(name+".DirectoryBuffer", b.numReqPerCycle)
	c.bankBufs = make([]sim.Buffer, b.numBank)

	for i := 0; i < b.numBank; i++ {
		c.bankBufs[i] = sim.NewBuffer(
			fmt.Sprintf("%s.Bank%d.Buffer", name, i),
			b.numReqPerCycle,
		)
	}

	c.mshr = cache.NewMSHR(b.numMSHREntry)
	blockSize := 1 << b.log2BlockSize
	numSets := int(b.totalByteSize / uint64(b.wayAssociativity*blockSize))
	c.directory = cache.NewDirectory(
		numSets, b.wayAssociativity, 1<<b.log2BlockSize,
		cache.NewLRUVictimFinder())
	c.storage = mem.NewStorage(b.totalByteSize)
	c.bankLatency = b.bankLatency
	c.wayAssociativity = b.wayAssociativity
	c.maxNumConcurrentTrans = b.maxNumConcurrentTrans

	b.configureAddressMapper(c)

	b.buildStages(c)

	if b.visTracer != nil {
		tracing.CollectTrace(c, b.visTracer)
	}

	middleware := &middleware{Comp: c}
	c.AddMiddleware(middleware)

	return c
}

func (b *Builder) buildStages(c *Comp) {
	c.coalesceStage = &coalescer{cache: c}
	b.buildDirStage(c)
	b.buildBankStages(c)
	c.parseBottomStage = &bottomParser{cache: c}
	c.respondStage = &respondStage{cache: c}

	c.controlStage = &controlStage{
		ctrlPort:     c.controlPort,
		transactions: &c.transactions,
		directory:    c.directory,
		cache:        c,
		bankStages:   c.bankStages,
		coalescer:    c.coalesceStage,
	}
}

func (b *Builder) buildDirStage(c *Comp) {
	buf := sim.NewBuffer(
		c.Name()+".DirectoryStage.PostPipelineBuffer",
		b.numReqPerCycle,
	)
	pipelineName := fmt.Sprintf("%s.Directory.Pipeline", c.Name())
	pipeline := pipelining.MakeBuilder().
		WithPipelineWidth(b.numReqPerCycle).
		WithNumStage(b.dirLatency).
		WithCyclePerStage(1).
		WithPostPipelineBuffer(buf).
		Build(pipelineName)
	c.directoryStage = &directory{
		cache:    c,
		buf:      buf,
		pipeline: pipeline,
	}
}

func (b *Builder) buildBankStages(c *Comp) {
	for i := 0; i < b.numBank; i++ {
		pipelineName := fmt.Sprintf("%s.Bank[%d].Pipeline", c.Name(), i)
		postPipelineBuf := sim.NewBuffer(
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", c.Name(), i),
			b.numReqPerCycle,
		)
		pipeline := pipelining.MakeBuilder().
			WithPipelineWidth(b.numReqPerCycle).
			WithNumStage(b.bankLatency).
			WithCyclePerStage(1).
			WithPostPipelineBuffer(postPipelineBuf).
			Build(pipelineName)
		bs := &bankStage{
			cache:           c,
			bankID:          i,
			numReqPerCycle:  b.numReqPerCycle,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
		c.bankStages = append(c.bankStages, bs)

		if b.visTracer != nil {
			tracing.CollectTrace(bs.pipeline, b.visTracer)
		}
	}
}

func (b *Builder) configureAddressMapper(c *Comp) {
	if b.addressToPortMapper != nil {
		c.addressToPortMapper = b.addressToPortMapper
		return
	}

	switch b.addressMapperType {
	case "single":
		if len(b.remotePorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		c.addressToPortMapper = &mem.SinglePortMapper{
			Port: b.remotePorts[0],
		}
	case "interleaved":
		if len(b.remotePorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		mapper := mem.NewInterleavedAddressPortMapper(4096)
		mapper.LowModules = append(mapper.LowModules, b.remotePorts...)
		c.addressToPortMapper = mapper
	default:
		panic("addressMapperType must be \"single\" or \"interleaved\"")
	}
}

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
