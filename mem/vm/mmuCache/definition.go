package mmuCache

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:            1 * timing.GHz,
		NumReqPerCycle:  4,
		NumLevels:       5,
		NumBlocks:       1,
		PageSize:        4096,
		LatencyPerLevel: 100,
		Log2PageSize:    12,
	}
}

func (s *state) Initialize(c *Comp) {
	spec := c.Spec

	if spec.NumBlocks <= 0 {
		panic("mmuCache: Spec.NumBlocks must be > 0")
	}

	*s = state{
		CurrentState:          mmuCacheStateEnable,
		Table:                 initSets(spec.NumLevels, spec.NumBlocks),
		OutstandingBottomReqs: map[uint64]bool{},
		InflightReqs:          map[uint64]inflightReqState{},
	}
}

func (m *middlewares) Initialize(c *Comp) {
	*m = middlewares{
		Ctrl:  &ctrlMiddleware{comp: c},
		Cache: &mmuCacheMiddleware{comp: c},
	}
}
