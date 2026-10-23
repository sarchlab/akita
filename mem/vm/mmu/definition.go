package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:                    1 * timing.GHz,
		Log2PageSize:            12,
		Latency:                 10,
		MaxInFlightTranslations: 16,
	}
}

func (m *middlewares) Initialize(c *Comp) {
	if c.Spec.Latency < 0 {
		panic("vm/mmu: Latency must be nonnegative")
	}
	pt := c.Resources.PageTable
	if pt == nil {
		panic("mmu: Resources.PageTable is required")
	}

	validatePageTablePageSize(pt, c.Spec.Log2PageSize)

	*m = middlewares{
		Ctrl:        &ctrlMiddleware{comp: c},
		Translation: &translationMW{comp: c},
	}
}

// validatePageTablePageSize checks if the provided page table's page size is
// consistent with the MMU's log2PageSize configuration.
func validatePageTablePageSize(pt *vm.PageTable, log2PageSize uint64) {
	if pt.Log2PageSize() != log2PageSize {
		panic("page table page size does not match MMU page size")
	}
}
