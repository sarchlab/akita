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
		Freq:                 1 * timing.GHz,
		NumReqPerCycle:       1,
		Log2BlockSize:        6,
		BankLatency:          10,
		NumWays:              4,
		NumBanks:             1,
		NumMSHREntries:       16,
		NumSets:              2048,
		WriteBufferCapacity:  1024,
		MaxInFlightFetches:   128,
		MaxInFlightEvictions: 128,
		DirectoryWidth:       2,
		BankWidth:            2,
		Bottom:               mem.AddressMapper{InterleavingSize: 4096},
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
	bankPipes := make([]queueing.FastPipeline[int], numBanks)
	bankPostBufs := make([]queueing.Buffer[int], numBanks)
	for i := 0; i < numBanks; i++ {
		dirToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		wbToBank[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)

		// A zero-latency stage bypasses its pipeline, which stays empty.
		bankPipes[i] = queueing.MakeFastPipeline[int](
			spec.BankWidth, max(spec.BankLatency, 1), spec.Freq, c)
		bankPostBufs[i] = queueing.MakeBuffer[int](spec.BankWidth)
	}

	initial := state{
		CacheState:   int(cacheStateRunning),
		EvictingList: make(map[uint64]bool),
		DirStageBuf:  queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirToBankBufs:         dirToBank,
		WriteBufferToBankBufs: wbToBank,
		MSHRStageBuf:          queueing.MakeBuffer[int](spec.NumReqPerCycle),

		WriteBufferBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		DirPipeline: queueing.MakeFastPipeline[int](
			spec.DirectoryWidth, max(spec.DirLatency, 1), dirStepFreq, dirStepClock{comp: c}),
		DirPostPipelineBuf: queueing.MakeBuffer[int](spec.DirectoryWidth),

		BankPipelines:                   bankPipes,
		BankPostPipelineBufs:            bankPostBufs,
		BankInflightTransCounts:         make([]int, numBanks),
		BankDownwardInflightTransCounts: make([]int, numBanks),
	}

	cache.DirectoryReset(&initial.DirectoryState,
		spec.NumSets, spec.NumWays, 1<<spec.Log2BlockSize)

	*s = initial
}

// AfterCheckpointLoad gives the decoded pipelines their clocks.
func (s *state) AfterCheckpointLoad(c *Comp) {
	s.DirPipeline.BindClock(dirStepClock{comp: c})
	for i := range s.BankPipelines {
		s.BankPipelines[i].BindClock(c)
	}
}

// dirStepFreq makes one picosecond of dirStepClock equal one directory step, so
// directory pipeline times are step numbers rather than simulation time.
const dirStepFreq = 1000 * timing.GHz

// dirStepClock is the directory pipeline's clock. The directory takes
// NumReqPerCycle steps per cycle, so its pipeline counts steps; the clock reads
// the step from the component's current State, which survives a restore.
type dirStepClock struct {
	comp *Comp
}

// CurrentTime returns the current directory step.
func (c dirStepClock) CurrentTime() timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(c.comp.State.DirStep)
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
	if spec.NumSets <= 0 || spec.NumWays <= 0 || spec.Log2BlockSize >= 63 {
		return fmt.Errorf("cache geometry must have positive sets and ways and a block exponent below 63")
	}
	if uint64(spec.NumSets) > (^uint64(0)>>spec.Log2BlockSize)/uint64(spec.NumWays) {
		return fmt.Errorf("cache byte size overflows uint64")
	}
	if spec.NumBanks <= 0 || spec.NumReqPerCycle <= 0 || spec.BankLatency < 0 || spec.DirLatency < 0 {
		return fmt.Errorf("cache banks and request width must be positive and latencies nonnegative")
	}
	if spec.DirectoryWidth <= 0 || spec.BankWidth <= 0 {
		return fmt.Errorf("pipeline widths must be positive")
	}
	if spec.BankWidth < 2 {
		return fmt.Errorf("writeback: BankWidth must be at least 2 to reserve an upward lane")
	}

	return spec.Bottom.Validate()
}
