package simplebankedmemory

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
)

// BankSelector decides which bank should serve a request.
type BankSelector func(req mem.AccessReq, numBanks int) int

// Builder constructs SimpleBankedMemory components.
type Builder struct {
	engine sim.Engine
	freq   sim.Freq

	numBanks            int
	bankQueueSize       int
	bankPipelineWidth   int
	bankPipelineDepth   int
	stageLatency        int
	topPortBufferSize   int
	postPipelineBufSize int

	bankSelector BankSelector
	bankSize     uint64

	capacity         uint64
	storage          *mem.Storage
	addressConverter mem.AddressConverter
}

// MakeBuilder creates a builder with reasonable defaults.
func MakeBuilder() Builder {
	return Builder{
		freq:                1 * sim.GHz,
		numBanks:            4,
		bankQueueSize:       32,
		bankPipelineWidth:   1,
		bankPipelineDepth:   1,
		stageLatency:        10,
		topPortBufferSize:   16,
		postPipelineBufSize: 32,
		bankSize:            64,
		capacity:            4 * mem.GB,
	}
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the component frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumBanks sets the number of banks.
func (b Builder) WithNumBanks(numBanks int) Builder {
	b.numBanks = numBanks
	return b
}

// WithBankQueueSize sets the queued request capacity per bank.
func (b Builder) WithBankQueueSize(size int) Builder {
	b.bankQueueSize = size
	return b
}

// WithBankPipelineWidth sets the pipeline width inside each bank.
func (b Builder) WithBankPipelineWidth(width int) Builder {
	b.bankPipelineWidth = width
	return b
}

// WithBankPipelineDepth sets the pipeline depth inside each bank.
func (b Builder) WithBankPipelineDepth(depth int) Builder {
	b.bankPipelineDepth = depth
	return b
}

// WithStageLatency sets the latency of each pipeline stage in cycles.
func (b Builder) WithStageLatency(latency int) Builder {
	b.stageLatency = latency
	return b
}

// WithTopPortBufferSize sets the buffer size of the top port.
func (b Builder) WithTopPortBufferSize(size int) Builder {
	b.topPortBufferSize = size
	return b
}

// WithPostPipelineBufferSize sets the post pipeline buffer capacity per bank.
func (b Builder) WithPostPipelineBufferSize(size int) Builder {
	b.postPipelineBufSize = size
	return b
}

// WithBankSelector overrides the default bank selector.
func (b Builder) WithBankSelector(selector BankSelector) Builder {
	b.bankSelector = selector
	return b
}

// WithBankSize sets the bank size used by the default selector.
func (b Builder) WithBankSize(bankSize uint64) Builder {
	b.bankSize = bankSize
	return b
}

// WithStorage reuses an existing storage object.
func (b Builder) WithStorage(storage *mem.Storage) Builder {
	b.storage = storage
	return b
}

// WithNewStorage creates a new storage with the given capacity.
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

// WithAddressConverter sets the address converter.
func (b Builder) WithAddressConverter(
	addressConverter mem.AddressConverter,
) Builder {
	b.addressConverter = addressConverter
	return b
}

// Build creates a SimpleBankedMemory component.
func (b Builder) Build(name string) *Comp {
	if b.engine == nil {
		panic("simplebankedmemory.Builder: engine is nil; call WithEngine")
	}

	if b.numBanks <= 0 {
		panic("simplebankedmemory.Builder: numBanks must be > 0")
	}

	if b.bankQueueSize <= 0 {
		panic("simplebankedmemory.Builder: bankQueueSize must be > 0")
	}

	if b.bankPipelineWidth <= 0 {
		panic("simplebankedmemory.Builder: bankPipelineWidth must be > 0")
	}

	if b.bankPipelineDepth <= 0 {
		panic("simplebankedmemory.Builder: bankPipelineDepth must be > 0")
	}

	if b.stageLatency <= 0 {
		panic("simplebankedmemory.Builder: stageLatency must be > 0")
	}

	if b.topPortBufferSize <= 0 {
		panic("simplebankedmemory.Builder: topPortBufferSize must be > 0")
	}

	if b.postPipelineBufSize <= 0 {
		panic("simplebankedmemory.Builder: postPipelineBufSize must be > 0")
	}

	if b.bankSelector == nil {
		b.bankSelector = makeDefaultBankSelector(b.bankSize)
	}

	var storage *mem.Storage
	if b.storage != nil {
		storage = b.storage
	} else {
		storage = mem.NewStorage(b.capacity)
	}

	c := &Comp{
		Storage:          storage,
		AddressConverter: b.addressConverter,
		bankSelector:     b.bankSelector,
	}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)

	c.topPort = sim.NewPort(c, b.topPortBufferSize, b.topPortBufferSize, name+".TopPort")
	c.AddPort("Top", c.topPort)

	c.banks = make([]bank, b.numBanks)

	for i := range c.banks {
		pending := sim.NewBuffer(
			fmt.Sprintf("%s.Bank[%d].Pending", name, i),
			b.bankQueueSize,
		)

		postPipelineBuf := sim.NewBuffer(
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", name, i),
			b.postPipelineBufSize,
		)

		pipeline := pipelining.MakeBuilder().
			WithPipelineWidth(b.bankPipelineWidth).
			WithNumStage(b.bankPipelineDepth).
			WithCyclePerStage(b.stageLatency).
			WithPostPipelineBuffer(postPipelineBuf).
			Build(fmt.Sprintf("%s.Bank[%d].Pipeline", name, i))

		c.banks[i] = bank{
			pending:         pending,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
	}

	c.AddMiddleware(&middleware{Comp: c})

	return c
}

func makeDefaultBankSelector(bankSize uint64) BankSelector {
	if bankSize == 0 {
		bankSize = 1
	}

	return func(req mem.AccessReq, numBanks int) int {
		if numBanks == 0 {
			return 0
		}

		addr := req.GetAddress()
		return int((addr / bankSize) % uint64(numBanks))
	}
}
