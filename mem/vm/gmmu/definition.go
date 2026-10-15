package gmmu

import (
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the GMMU, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		MaxRequestsInFlight: 16,
	},
}

func (initialState *state) Initialize(_ *Comp) {
	*initialState = state{
		RemoteMemReqs: make(map[uint64]transactionState),
	}
}

func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	pt := c.Resources.PageTable
	if pt == nil {
		panic("gmmu: Resources.PageTable is required")
	}

	*initialMiddlewares = middlewares{
		Ctrl:    &ctrlMiddleware{comp: c},
		Walk:    &walkMW{comp: c, pageTable: pt},
		Respond: &respondMW{comp: c},
	}
}
