package tlb

import (
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the TLB, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		NumSets:        1,
		NumWays:        32,
		Log2PageSize:   12,
		MSHRSize:       4,
		Latency:        4,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(c *Comp) state {
	spec := c.Spec

	return state{
		TLBState: tlbStateEnable,
		Sets:     initSets(spec.NumSets, spec.NumWays),
		Pipeline: queueing.MakePipeline[pipelineTLBReqState](
			spec.NumReqPerCycle,
			spec.Latency),

		BufferItems: queueing.MakeBuffer[pipelineTLBReqState](
			spec.NumReqPerCycle),
	}
}

func newMiddlewares(c *Comp) middlewares {
	return middlewares{
		Ctrl: &ctrlMiddleware{comp: c},
		TLB:  &tlbMiddleware{comp: c},
	}
}
