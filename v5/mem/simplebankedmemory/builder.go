package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides default configuration for the simple banked memory.
var DefaultSpec = Spec{
	Freq:                           1 * sim.GHz,
	NumBanks:                       4,
	BankPipelineWidth:              1,
	BankPipelineDepth:              1,
	StageLatency:                   10,
	PostPipelineBufSize:            1,
	BankSelectorKind:               "interleaved",
	BankSelectorLog2InterleaveSize: 6,
}

// Builder constructs SimpleBankedMemory components.
type Builder struct {
	engine sim.EventScheduler
	spec   Spec

	numBanks            int
	bankPipelineWidth   int
	bankPipelineDepth   int
	stageLatency        int
	topPortBufferSize   int
	postPipelineBufSize int

	bankSelectorKind   string
	log2InterleaveSize uint64

	addrConvKind            string
	addrInterleavingSize    uint64
	addrTotalNumOfElements  int
	addrCurrentElementIndex int
	addrOffset              uint64

	capacity uint64
	storage  *mem.Storage
	topPort  sim.Port
}

// MakeBuilder creates a builder with reasonable defaults.
func MakeBuilder() Builder {
	return Builder{
		spec:                DefaultSpec,
		numBanks:            DefaultSpec.NumBanks,
		bankPipelineWidth:   DefaultSpec.BankPipelineWidth,
		bankPipelineDepth:   DefaultSpec.BankPipelineDepth,
		stageLatency:        DefaultSpec.StageLatency,
		topPortBufferSize:   16,
		postPipelineBufSize: DefaultSpec.PostPipelineBufSize,
		bankSelectorKind:    DefaultSpec.BankSelectorKind,
		log2InterleaveSize:  DefaultSpec.BankSelectorLog2InterleaveSize,
		capacity:            4 * mem.GB,
	}
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the component frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNumBanks sets the number of banks.
func (b Builder) WithNumBanks(numBanks int) Builder {
	b.numBanks = numBanks
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

// WithPostPipelineBufferSize sets the post-pipeline buffer capacity per bank.
func (b Builder) WithPostPipelineBufferSize(size int) Builder {
	b.postPipelineBufSize = size
	return b
}

// WithBankSelectorType selects the bank selector implementation by name.
func (b Builder) WithBankSelectorType(selectorType string) Builder {
	b.bankSelectorKind = selectorType
	return b
}

// WithLog2InterleaveSize sets the log2 interleave size used by the default
// selector.
func (b Builder) WithLog2InterleaveSize(log2Size uint64) Builder {
	b.log2InterleaveSize = log2Size
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

// WithAddressConverter sets the address converter, inlining the configuration
// into the Spec if it is an InterleavingConverter.
func (b Builder) WithAddressConverter(
	addressConverter mem.AddressConverter,
) Builder {
	if ic, ok := addressConverter.(mem.InterleavingConverter); ok {
		b.addrConvKind = "interleaving"
		b.addrInterleavingSize = ic.InterleavingSize
		b.addrTotalNumOfElements = ic.TotalNumOfElements
		b.addrCurrentElementIndex = ic.CurrentElementIndex
		b.addrOffset = ic.Offset
	}

	return b
}

// WithTopPort sets the top port of the memory component.
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// Build creates a SimpleBankedMemory component.
func (b Builder) Build(name string) *Comp {
	b.configurationMustBeValid()

	storage := b.resolveStorage()
	spec := b.buildSpec(name)
	initialState := b.buildInitialState(spec)

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	c := &Comp{
		Component: modelComp,
		storage:   storage,
	}

	b.topPort.SetComponent(c)
	modelComp.AddPort("Top", b.topPort)

	tfMW := &tickFinalizeMW{comp: modelComp, storage: storage}
	modelComp.AddMiddleware(tfMW)
	dMW := &dispatchMW{comp: modelComp}
	modelComp.AddMiddleware(dMW)

	return c
}

func (b Builder) resolveStorage() *mem.Storage {
	if b.storage != nil {
		return b.storage
	}

	return mem.NewStorage(b.capacity)
}

func (b Builder) buildSpec(name string) Spec {
	return Spec{
		Freq:                           b.spec.Freq,
		NumBanks:                       b.numBanks,
		BankPipelineWidth:              b.bankPipelineWidth,
		BankPipelineDepth:              b.bankPipelineDepth,
		StageLatency:                   b.stageLatency,
		PostPipelineBufSize:            b.postPipelineBufSize,
		BankSelectorKind:               b.bankSelectorKind,
		BankSelectorLog2InterleaveSize: b.log2InterleaveSize,
		AddrConvKind:                   b.addrConvKind,
		AddrInterleavingSize:           b.addrInterleavingSize,
		AddrTotalNumOfElements:         b.addrTotalNumOfElements,
		AddrCurrentElementIndex:        b.addrCurrentElementIndex,
		AddrOffset:                     b.addrOffset,
		StorageRef:                     name,
	}
}

func (b Builder) buildInitialState(spec Spec) State {
	state := State{
		Banks: make([]bankState, spec.NumBanks),
	}

	for i := range state.Banks {
		state.Banks[i] = bankState{
			Pipeline: queueing.Pipeline[bankPipelineItemState]{
				Width:     spec.BankPipelineWidth,
				NumStages: spec.BankPipelineDepth * spec.StageLatency,
			},
			PostPipelineBuf: nil,
		}
	}

	return state
}

func (b Builder) configurationMustBeValid() {
	if b.engine == nil {
		panic("simplebankedmemory.Builder: engine is nil; call WithEngine")
	}

	if b.numBanks <= 0 {
		panic("simplebankedmemory.Builder: numBanks must be > 0")
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
}
