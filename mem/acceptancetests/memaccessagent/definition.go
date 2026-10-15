package memaccessagent

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:       1 * timing.GHz,
		MaxAddress: 1024 * 1024,
		WriteLeft:  1000,
		ReadLeft:   1000,
		RandSeed:   1,
	}
}

func (s *State) Initialize(c *Comp) {
	spec := c.Spec

	*s = State{
		WriteLeft:       spec.WriteLeft,
		ReadLeft:        spec.ReadLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]pendingRequest),
		PendingWriteReq: make(map[uint64]pendingRequest),
		RNG:             newRandState(spec.RandSeed),
	}
}

func (m *Middlewares) Initialize(c *Comp) {
	if c.Spec.LowModule == "" {
		panic("memaccessagent: Spec.LowModule is required")
	}

	*m = Middlewares{
		Agent: &agentMiddleware{comp: c},
	}
}
