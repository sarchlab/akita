package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:                            1 * timing.GHz,
		NumReqPerCycle:                  1,
		Log2BlockSize:                   6,
		BankLatency:                     10,
		WayAssociativity:                4,
		NumBanks:                        1,
		NumMSHREntry:                    16,
		NumSets:                         2048,
		WriteBufferCapacity:             1024,
		MaxInflightFetch:                128,
		MaxInflightEviction:             128,
		DirBufferCapacity:               1,
		DirToBankBufferCapacity:         1,
		WriteBufferToBankBufferCapacity: 1,
		MSHRBufferCapacity:              1,
		WriteBufferStageCapacity:        1,
		DirPostBufferCapacity:           1,
		BankPostBufferCapacity:          2,
		DirectoryWidth:                  2,
		BankWidth:                       2,
		Bottom:                          mem.AddressMapper{InterleavingSize: 4096},
	}
}

// Initialize creates a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func (s *state) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	spec := c.Spec

	numBanks := spec.NumBanks

	dirToBank := make([]queueing.Buffer[int], numBanks)
	wbToBank := make([]queueing.Buffer[int], numBanks)
	bankPipes := make([]queueing.Pipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.MakeBuffer[int](spec.DirToBankBufferCapacity)

		wbToBank[i] = queueing.MakeBuffer[int](spec.WriteBufferToBankBufferCapacity)

		bankPipes[i] = queueing.MakePipeline[int](spec.BankWidth, spec.BankLatency)
		bankPostBufs[i] = queueing.MakeBuffer[int](spec.BankPostBufferCapacity)
	}

	initial := state{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf:  queueing.MakeBuffer[int](spec.DirBufferCapacity),

		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf:          queueing.MakeBuffer[int](spec.MSHRBufferCapacity),

		WriteBufferBuf: queueing.MakeBuffer[int](spec.WriteBufferStageCapacity),

		DirPipeline:        queueing.MakePipeline[int](spec.DirectoryWidth, spec.DirLatency),
		DirPostPipelineBuf: queueing.MakeBuffer[int](spec.DirPostBufferCapacity),

		BankPipelines:                   bankPipes,
		BankPostPipelineBufs:            bankPostBufs,
		BankInflightTransCounts:         make([]int, numBanks),
		BankDownwardInflightTransCounts: make([]int, numBanks),
	}

	cache.DirectoryReset(&initial.DirectoryState,
		spec.NumSets, spec.WayAssociativity, 1<<spec.Log2BlockSize)

	*s = initial
}

// Initialize creates the cache's middlewares. The control middlewares and
// the pipeline stages share the pipeline middleware, which holds the storage,
// and the ports' accessors.
func (m *middlewares) Initialize(c *Comp) {
	res := c.Resources
	if res.Storage == nil {
		panic("writeback: Resources.Storage is required")
	}

	pipeline := &pipelineMW{
		comp:    c,
		storage: res.Storage,
	}
	pipeline.createInternalStages()

	*m = middlewares{
		Ctrl: &ctrlMiddleware{pipeline: pipeline},
		Flusher: &controlMW{
			comp:    c,
			flusher: &flusher{pipeline: pipeline},
		},
		Pipeline: pipeline,
	}
}

func validateSpec(spec Spec) error {
	if spec.NumSets <= 0 || spec.WayAssociativity <= 0 || spec.Log2BlockSize >= 63 {
		return fmt.Errorf("cache geometry must have positive sets and ways and a block exponent below 63")
	}
	if uint64(spec.NumSets) > (^uint64(0)>>spec.Log2BlockSize)/uint64(spec.WayAssociativity) {
		return fmt.Errorf("cache byte size overflows uint64")
	}
	if spec.NumBanks <= 0 || spec.NumReqPerCycle <= 0 || spec.BankLatency < 0 || spec.DirLatency < 0 {
		return fmt.Errorf("cache banks and request width must be positive and latencies nonnegative")
	}
	if spec.DirBufferCapacity <= 0 ||
		spec.DirToBankBufferCapacity <= 0 ||
		spec.WriteBufferToBankBufferCapacity <= 0 ||
		spec.MSHRBufferCapacity <= 0 ||
		spec.WriteBufferStageCapacity <= 0 ||
		spec.DirPostBufferCapacity <= 0 ||
		spec.BankPostBufferCapacity <= 0 ||
		spec.DirectoryWidth <= 0 ||
		spec.BankWidth <= 0 {
		return fmt.Errorf("cache pipeline widths and buffer capacities must be positive")
	}
	if spec.BankWidth < 2 {
		return fmt.Errorf("writeback: BankWidth must be at least 2 to reserve an upward lane")
	}

	return spec.Bottom.Validate()
}
