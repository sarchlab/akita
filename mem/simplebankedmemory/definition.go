package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:                           1 * timing.GHz,
		NumBanks:                       4,
		BankPipelineWidth:              1,
		BankPipelineDepth:              1,
		StageLatency:                   10,
		PostPipelineBufSize:            1,
		BankSelectorLog2InterleaveSize: 6,
	}
}

func (s *state) Initialize(c *Comp) {
	name, spec := c.Name(), c.Spec

	*s = state{
		Banks: buildInitialBanks(name, spec),
	}
}

func (m *middlewares) Initialize(c *Comp) {
	if c.Resources.Storage == nil {
		panic("simplebankedmemory: Resources.Storage is required")
	}

	*m = middlewares{
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

			PostPipelineBuf: queueing.MakeBuffer[bankPipelineItemState](spec.PostPipelineBufSize),
		}
	}

	return banks
}
