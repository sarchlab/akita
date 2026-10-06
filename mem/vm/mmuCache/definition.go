package mmuCache

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the MMUCache, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:            1 * timing.GHz,
		NumReqPerCycle:  4,
		NumLevels:       5,
		NumBlocks:       1,
		PageSize:        4096,
		LatencyPerLevel: 100,
		Log2PageSize:    12,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(c *Comp) state {
	spec := c.Spec

	if spec.NumBlocks <= 0 {
		panic("mmuCache: Spec.NumBlocks must be > 0")
	}

	return state{
		CurrentState:          mmuCacheStateEnable,
		Table:                 initSets(spec.NumLevels, spec.NumBlocks),
		OutstandingBottomReqs: map[uint64]bool{},
		InflightReqs:          map[uint64]inflightReqState{},
	}
}

func newMiddlewares(c *Comp) middlewares {
	return middlewares{
		Ctrl:  &ctrlMiddleware{comp: c},
		Cache: &mmuCacheMiddleware{comp: c},
	}
}
