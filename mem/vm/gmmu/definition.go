package gmmu

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		MaxRequestsInFlight: 16,
	}
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
