package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the writeback cache, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		NumReqPerCycle:      1,
		Log2BlockSize:       6,
		BankLatency:         10,
		WayAssociativity:    4,
		NumBanks:            1,
		NumMSHREntry:        16,
		TotalByteSize:       512 * mem.KB,
		WriteBufferCapacity: 1024,
		MaxInflightFetch:    128,
		MaxInflightEviction: 128,
		Bottom:              mem.AddressMapper{InterleavingSize: 4096},
	},
}

// Initialize creates a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func (initialState *state) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(err)
	}

	spec := c.Spec

	numBanks := spec.numBanks()
	laneWidth := spec.laneWidth()

	dirToBank := make([]queueing.Buffer[int], numBanks)
	wbToBank := make([]queueing.Buffer[int], numBanks)
	bankPipes := make([]queueing.Pipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		wbToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		bankPipes[i] = queueing.MakePipeline[int](laneWidth, spec.BankLatency)
		bankPostBufs[i] = queueing.MakeBuffer[int](laneWidth)
	}

	s := state{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf:  queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf:          queueing.MakeBuffer[int](spec.NumReqPerCycle),

		WriteBufferBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirPipeline:        queueing.MakePipeline[int](laneWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankPipelines:                   bankPipes,
		BankPostPipelineBufs:            bankPostBufs,
		BankInflightTransCounts:         make([]int, numBanks),
		BankDownwardInflightTransCounts: make([]int, numBanks),
	}

	cache.DirectoryReset(&s.DirectoryState,
		spec.numSets(), spec.WayAssociativity, 1<<spec.Log2BlockSize)

	*initialState = s
}

// Initialize creates the cache's middlewares. The control middlewares and
// the pipeline stages share the pipeline middleware, which holds the storage,
// and the ports' accessors.
func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	res := c.Resources
	if res.Storage == nil {
		panic("writeback: Resources.Storage is required")
	}

	pipeline := &pipelineMW{
		comp:    c,
		storage: res.Storage,
	}
	pipeline.createInternalStages()

	*initialMiddlewares = middlewares{
		Ctrl: &ctrlMiddleware{pipeline: pipeline},
		Flusher: &controlMW{
			comp:    c,
			flusher: &flusher{pipeline: pipeline},
		},
		Pipeline: pipeline,
	}
}

func validateSpec(spec Spec) error {
	return spec.Bottom.Validate()
}
