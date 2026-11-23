package writethrough

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
		Freq:                1 * timing.GHz,
		NumReqPerCycle:      4,
		Log2BlockSize:       6,
		BankLatency:         20,
		NumWays:             4,
		MaxInFlightRequests: 16,
		NumBanks:            1,
		NumMSHREntries:      4,
		NumSets:             16,
		DirLatency:          2,
		WritePolicy:         WritePolicyWriteAround,
		DirectoryWidth:      4,
		BankWidth:           4,
		Bottom:              mem.AddressMapper{InterleavingSize: 4096},
	}
}

// Initialize creates a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func (s *state) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	spec := c.Spec

	bankBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankBufs[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)
	}

	// A zero-latency stage bypasses its pipeline, which stays empty.
	bankPipelines := make([]queueing.FastPipeline[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPipelines[i] = queueing.MakeFastPipeline[int](
			spec.BankWidth,
			max(spec.BankLatency, 1),
			spec.Freq,
			c)
	}

	bankPostBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPostBufs[i] = queueing.MakeBuffer[int](spec.BankWidth)
	}

	initial := state{
		DirBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankBufs: bankBufs,
		DirPipeline: queueing.MakeFastPipeline[int](
			spec.DirectoryWidth,
			max(spec.DirLatency, 1),
			dirStepFreq,
			dirStepClock{comp: c}),

		DirPostBuf: queueing.MakeBuffer[int](spec.DirectoryWidth),

		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&initial.DirectoryState, spec.NumSets, spec.NumWays,
		spec.blockSize())

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

// dirStepClock is the directory pipeline's clock. The directory admits at step
// c and drains at step c+1 within cycle c; the clock reads the step from the
// component's current State, which survives a restore.
type dirStepClock struct {
	comp *Comp
}

// CurrentTime returns the current directory step.
func (c dirStepClock) CurrentTime() timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(c.comp.State.DirStep)
}

// Initialize creates the cache's middlewares. The pipeline middleware
// holds the storage and the pipeline stages.
func (m *middlewares) Initialize(c *Comp) {
	res := c.Resources
	if res.Storage == nil {
		panic("writethrough: Resources.Storage is required")
	}

	*m = middlewares{
		Ctrl: &ctrlMiddleware{comp: c},
		Pipeline: newPipelineMW(
			c, res.Storage),
	}
}

func validateSpec(spec Spec) error {
	switch spec.writePolicy() {
	case WritePolicyWriteAround, WritePolicyWriteEvict, WritePolicyWriteThrough:
	default:
		return fmt.Errorf("unsupported write policy %q", spec.WritePolicy)
	}
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

	return spec.Bottom.Validate()
}
