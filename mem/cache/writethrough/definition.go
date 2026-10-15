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
		Freq:                  1 * timing.GHz,
		NumReqPerCycle:        4,
		Log2BlockSize:         6,
		BankLatency:           20,
		WayAssociativity:      4,
		MaxNumConcurrentTrans: 16,
		NumBanks:              1,
		NumMSHREntry:          4,
		TotalByteSize:         4 * mem.KB,
		DirLatency:            2,
		WritePolicyType:       "write-around",
		Bottom:                mem.AddressMapper{InterleavingSize: 4096},
	}
}

// Initialize creates a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func (initialState *state) Initialize(c *Comp) {
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
			spec.NumReqPerCycle,
			spec.BankLatency)
	}

	bankPostBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPostBufs[i] = queueing.MakeBuffer[int](spec.NumReqPerCycle)
	}

	s := state{
		DirBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankBufs: bankBufs,
		DirPipeline: queueing.MakePipeline[int](
			spec.NumReqPerCycle,
			spec.DirLatency),

		DirPostBuf: queueing.MakeBuffer[int](spec.NumReqPerCycle),

		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&s.DirectoryState, spec.numSets(), spec.WayAssociativity,
		spec.blockSize())

	*initialState = s
}

// Initialize creates the cache's middlewares. The pipeline middleware
// holds the storage and the pipeline stages.
func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	res := c.Resources
	if res.Storage == nil {
		panic("writethrough: Resources.Storage is required")
	}

	*initialMiddlewares = middlewares{
		Ctrl: &ctrlMiddleware{comp: c},
		Pipeline: newPipelineMW(
			c, res.Storage),
	}
}

func validateSpec(spec Spec) error {
	return spec.Bottom.Validate()
}
