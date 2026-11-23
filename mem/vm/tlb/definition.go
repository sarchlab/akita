package tlb

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		NumSets:        1,
		NumWays:        32,
		Log2PageSize:   12,
		NumMSHREntries: 4,
		Latency:        4,
	}
}

func (s *state) Initialize(c *Comp) {
	if err := validateSpec(c.Spec); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}

	spec := c.Spec

	*s = state{
		TLBState: tlbStateEnable,
		Sets:     initSets(spec.NumSets, spec.NumWays),
		// A zero-latency lookup bypasses the pipeline, which stays empty.
		Pipeline: queueing.MakeFastPipeline[pipelineTLBReqState](
			spec.NumReqPerCycle,
			max(spec.Latency, 1),
			spec.Freq,
			c),

		BufferItems: queueing.MakeBuffer[pipelineTLBReqState](spec.NumReqPerCycle),
	}
}

// AfterCheckpointLoad gives the decoded pipeline its clock.
func (s *state) AfterCheckpointLoad(c *Comp) {
	s.Pipeline.BindClock(c)
}

func (m *middlewares) Initialize(c *Comp) {
	*m = middlewares{
		Ctrl: &ctrlMiddleware{comp: c},
		TLB:  &tlbMiddleware{comp: c},
	}
}

func validateSpec(spec Spec) error {
	if spec.Latency < 0 {
		return fmt.Errorf("TLB latency must be nonnegative")
	}
	if spec.NumReqPerCycle <= 0 {
		return fmt.Errorf("TLB processing width must be positive")
	}
	return spec.Bottom.Validate()
}
