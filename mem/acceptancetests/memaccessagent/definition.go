package memaccessagent

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
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
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(c *Comp) State {
	spec := c.Spec

	return State{
		WriteLeft:       spec.WriteLeft,
		ReadLeft:        spec.ReadLeft,
		KnownMemValue:   make(map[uint64][]uint32),
		PendingReadReq:  make(map[uint64]messaging.Msg),
		PendingWriteReq: make(map[uint64]messaging.Msg),
		RNG:             newRandState(spec.RandSeed),
	}
}

func newMiddlewares(c *Comp) Middlewares {
	if c.Resources.LowModule == nil {
		panic("memaccessagent: Resources.LowModule is required")
	}

	return Middlewares{
		Agent: &agentMiddleware{comp: c},
	}
}
