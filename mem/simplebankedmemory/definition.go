package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the SimpleBankedMemory, a ticking component: its
// default configuration and its behavior. Its ports and middlewares are the
// fields of Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:                           1 * timing.GHz,
		NumBanks:                       4,
		BankPipelineWidth:              1,
		BankPipelineDepth:              1,
		StageLatency:                   10,
		PostPipelineBufSize:            1,
		Capacity:                       4 * mem.GB,
		BankSelectorKind:               "interleaved",
		BankSelectorLog2InterleaveSize: 6,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(c *Comp) state {
	name, spec := c.Name(), c.Spec

	return state{
		Banks: buildInitialBanks(name, spec),
	}
}

func newMiddlewares(c *Comp) middlewares {
	if c.Resources.Storage == nil {
		panic("simplebankedmemory: Resources.Storage is required")
	}

	return middlewares{
		Ctrl:         &ctrlMiddleware{comp: c},
		TickFinalize: &tickFinalizeMW{comp: c},
		Dispatch:     &dispatchMW{comp: c},
	}
}

// buildInitialBanks returns the empty banks of the memory named name.
func buildInitialBanks(name string, spec Spec) []bankState {
	banks := make([]bankState, spec.NumBanks)
	for i := range banks {
		banks[i] = bankState{
			Pipeline: queueing.MakePipeline[bankPipelineItemState](
				spec.BankPipelineWidth,
				spec.BankPipelineDepth*spec.StageLatency),

			PostPipelineBuf: queueing.MakeBuffer[bankPipelineItemState](
				spec.PostPipelineBufSize),
		}
	}

	return banks
}
