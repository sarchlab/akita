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

	bankPipelines := make([]queueing.Pipeline[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPipelines[i] = queueing.MakePipeline[int](
			spec.BankWidth,
			spec.BankLatency)
	}

	bankPostBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPostBufs[i] = queueing.MakeBuffer[int](spec.BankWidth)
	}

	initial := state{
		DirBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankBufs: bankBufs,
		DirPipeline: queueing.MakePipeline[int](
			spec.DirectoryWidth,
			spec.DirLatency),

		DirPostBuf: queueing.MakeBuffer[int](spec.DirectoryWidth),

		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&initial.DirectoryState, spec.NumSets, spec.NumWays,
		spec.blockSize())

	*s = initial
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
