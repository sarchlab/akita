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
	spec := c.Spec
	if spec.StageLatency < 0 {
		panic("simplebankedmemory: StageLatency must be nonnegative")
	}
	spec.BankAddressConversion.validate()

	*s = state{
		Banks: buildInitialBanks(c),
	}
}

// AfterCheckpointLoad gives the decoded bank pipelines their clock.
func (s *state) AfterCheckpointLoad(c *Comp) {
	for i := range s.Banks {
		s.Banks[i].Pipeline.BindClock(c)
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
		Wake:         &wakeMW{comp: c},
	}
}

// buildInitialBanks returns the empty banks of the memory c.
func buildInitialBanks(c *Comp) []bankState {
	spec := c.Spec
	width, numStages := bankPipelineShape(spec)
	banks := make([]bankState, spec.NumBanks)
	for i := range banks {
		banks[i] = bankState{
			Pipeline: queueing.MakeFastPipeline[bankPipelineItemState](width, numStages, spec.Freq, c),
		}
	}

	return banks
}
