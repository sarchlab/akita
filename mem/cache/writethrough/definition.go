package writethrough

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the writethrough, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		AddressMapperType:     mem.AddressMapperSingle,
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
		InterleavingSize:      4096,
		WritePolicyType:       "write-around",
	},
	NewState:       newState,
	ValidateSpec:   validateSpec,
	NewMiddlewares: newMiddlewares,
}

// newState returns a running cache with an empty directory, MSHR, and
// transaction table, and empty stage buffers and pipelines.
func newState(c *Comp) state {
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

	return s
}

// newMiddlewares creates the cache's middlewares. The pipeline middleware
// holds the storage, the address mapper, and the pipeline stages.
func newMiddlewares(c *Comp) middlewares {
	res := c.Resources
	if res.Storage == nil {
		panic("writethrough: Resources.Storage is required")
	}

	return middlewares{
		Ctrl: &ctrlMiddleware{comp: c},
		Pipeline: newPipelineMW(
			c, res.Storage, resolveAddressMapper(c.Spec)),
	}
}

// resolveAddressMapper derives the lookup solely from immutable Spec data.
func resolveAddressMapper(spec Spec) mem.AddressToPortMapper { return spec.addressMapping().Mapper() }

func validateSpec(spec Spec) error {
	return spec.addressMapping().Validate()
}
