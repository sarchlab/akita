package memaccessagent

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the MemAccessAgent, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:       1 * timing.GHz,
		MaxAddress: 1024 * 1024,
		WriteLeft:  1000,
		ReadLeft:   1000,
		RandSeed:   1,
	},
}

func (initialState *State) Initialize(c *Comp) {
	spec := c.Spec

	*initialState = State{
		WriteLeft:       spec.WriteLeft,
		ReadLeft:        spec.ReadLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]pendingRequest),
		PendingWriteReq: make(map[uint64]pendingRequest),
		RNG:             newRandState(spec.RandSeed),
	}
}

func (initialMiddlewares *Middlewares) Initialize(c *Comp) {
	if c.Spec.LowModule == "" {
		panic("memaccessagent: Spec.LowModule is required")
	}

	*initialMiddlewares = Middlewares{
		Agent: &agentMiddleware{comp: c},
	}
}
