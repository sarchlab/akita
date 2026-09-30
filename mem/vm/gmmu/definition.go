package gmmu

import (
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the GMMU, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		MaxRequestsInFlight: 16,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(_ *Comp) State {
	return State{
		RemoteMemReqs: make(map[uint64]transactionState),
	}
}

func newMiddlewares(c *Comp) Middlewares {
	pt := c.Resources().PageTable
	if pt == nil {
		panic("gmmu: Resources.PageTable is required")
	}

	return Middlewares{
		Ctrl:    &ctrlMiddleware{comp: c},
		Walk:    &walkMW{comp: c, pageTable: pt},
		Respond: &respondMW{comp: c},
	}
}
