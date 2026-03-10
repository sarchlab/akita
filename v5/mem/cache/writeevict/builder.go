package writeevict

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
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
		wayAssociativity:      2,
		numMSHREntry:          4,
		numBank:               1,
		maxNumConcurrentTrans: 16,
		numReqPerCycle:        4,
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
func (b *Builder) WithMaxNumConcurrentTrans(n int) *Builder {
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

// Build returns a new cache unit
func (b Builder) Build(name string) *Comp {
	b.assertAllRequiredInformationIsAvailable()

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(Spec{}).
		Build(name)

	c := &Comp{
		Component:      modelComp,
		log2BlockSize:  b.log2BlockSize,
		numReqPerCycle: b.numReqPerCycle,
	}

	b.createPorts(c)

	c.dirBuf = queueing.NewBuffer(
		c.Name()+".DirBuf",
		b.numReqPerCycle,
	)
	c.bankBufs = make([]queueing.Buffer, b.numBank)

	for i := 0; i < b.numBank; i++ {
		c.bankBufs[i] = queueing.NewBuffer(
			c.Name()+".BankBuf"+fmt.Sprint(i),
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
	c.addressToPortMapper = b.addressToPortMapper
	c.maxNumConcurrentTrans = b.maxNumConcurrentTrans

	b.buildStages(c)

	if b.visTracer != nil {
		tracing.CollectTrace(c, b.visTracer)
	}

	middleware := &middleware{Comp: c}
	c.AddMiddleware(middleware)

	return c
}

func (b *Builder) createPorts(cache *Comp) {
	cache.topPort = b.topPort
	cache.topPort.SetComponent(cache)
	cache.AddPort("Top", cache.topPort)

	cache.bottomPort = b.bottomPort
	cache.bottomPort.SetComponent(cache)
	cache.AddPort("Bottom", cache.bottomPort)

	cache.controlPort = b.controlPort
	cache.controlPort.SetComponent(cache)
	cache.AddPort("Control", cache.controlPort)
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
	buf := queueing.NewBuffer(
		c.Name()+".Directory.PostPipelineBuffer",
		b.numReqPerCycle,
	)
	pipelineName := fmt.Sprintf("%s.Directory.Pipeline", c.Name())
	pipeline := queueing.MakeBuilder().
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
		postPipelineBuf := queueing.NewBuffer(
			c.Name()+".BankBuf"+fmt.Sprint(i),
			b.numReqPerCycle,
		)
		pipeline := queueing.MakeBuilder().
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

func (b *Builder) assertAllRequiredInformationIsAvailable() {
	if b.engine == nil {
		panic("engine is not specified")
	}
}
