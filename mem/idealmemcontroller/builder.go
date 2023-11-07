package idealmemcontroller

import (
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
)

type Builder struct {
	width             uint64
	latency           int
	maxNumTransaction int
	freq              sim.Freq
	capacity          uint64
	engine            sim.Engine
	clsize            int
	topBufSize        int

	numCyclePerStage int
	numStage         int
	clPerCycle       int
	pipeBufSize      int
}

func MakeBuilder() Builder {
	return Builder{
		width:             64,
		latency:           100,
		maxNumTransaction: 8,
		freq:              1 * sim.GHz,
		capacity:          4 * mem.GB,
		clsize:            64,
		clPerCycle:        1,
		topBufSize:        16,
		numCyclePerStage:  0,
		numStage:          0,
		pipeBufSize:       16,
	}
}

func (b Builder) WithWidth(width uint64) Builder {
	b.width = width
	return b
}

func (b Builder) WithLatency(latency int) Builder {
	b.latency = latency
	return b
}

func (b Builder) WithMaxNumTransaction(maxNumTransaction int) Builder {
	b.maxNumTransaction = maxNumTransaction
	return b
}

func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

func (b Builder) WithCapacity(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

func (b Builder) WithClSize(clsize int) Builder {
	b.clsize = clsize
	return b
}

func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

func (b Builder) WithNumCyclePerStage(numCyclePerStage int) Builder {
	b.numCyclePerStage = numCyclePerStage
	return b
}

func (b Builder) WithNumStage(numStage int) Builder {
	b.numStage = numStage
	return b
}

func (b Builder) WithClPerCycle(clPerCycle int) Builder {
	b.clPerCycle = clPerCycle
	return b
}

func (b Builder) WithPipeBufSize(pipeBufSize int) Builder {
	b.pipeBufSize = pipeBufSize
	return b
}

func (b Builder) WithTopBufSize(topBufSize int) Builder {
	b.topBufSize = topBufSize
	return b
}

// New creates a new ideal memory controller
func (b Builder) Build(
	name string,
) *Comp {
	c := &Comp{
		Latency:           b.latency,
		MaxNumTransaction: b.maxNumTransaction,
		width:             b.clPerCycle,
		numCyclePerStage:  b.numCyclePerStage,
		numStage:          b.numStage,

		// clsize:            b.clsize,

	}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)
	c.Latency = b.latency
	c.MaxNumTransaction = b.maxNumTransaction

	c.Storage = mem.NewStorage(b.capacity)

	c.postPipelineBuf = sim.NewBuffer(c.Name()+
		".PostPipelineBuf",
		b.pipeBufSize)

	c.pipeline = pipelining.MakeBuilder().
		WithNumStage(b.numStage).
		WithCyclePerStage(b.numCyclePerStage).
		WithPipelineWidth(b.clPerCycle).
		WithPostPipelineBuffer(c.postPipelineBuf).
		Build(c.Name() + ".Pipeline")

	c.topPort = sim.NewLimitNumMsgPort(c, b.topBufSize, name+".TopPort")
	c.AddPort("Top", c.topPort)

	return c
}
