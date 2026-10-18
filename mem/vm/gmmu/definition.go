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

func (s *state) Initialize(_ *Comp) {
	*s = state{
		RemoteMemReqs: make(map[uint64]transactionState),
	}
}

func (m *middlewares) Initialize(c *Comp) {
	pt := c.Resources.PageTable
	if pt == nil {
		panic("gmmu: Resources.PageTable is required")
	}

	if pt.Log2PageSize() != c.Spec.Log2PageSize {
		panic("page table page size does not match GMMU page size")
	}

	*m = middlewares{
		Ctrl:    &ctrlMiddleware{comp: c},
		Walk:    &walkMW{comp: c, pageTable: pt},
		Respond: &respondMW{comp: c},
	}
}
