package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the MMU, a ticking component: its default configuration
// and its behavior. Its ports and middlewares are the fields of Ports and
// Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name), supplying Resources.PageTable; tooling
// reads the same declaration statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		Latency:             10,
		MaxRequestsInFlight: 16,
	},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) middlewares {
	pt := c.Resources().PageTable
	if pt == nil {
		panic("mmu: Resources.PageTable is required")
	}

	validatePageTablePageSize(pt, c.Spec().Log2PageSize)

	return middlewares{
		Ctrl:        &ctrlMiddleware{comp: c},
		Translation: &translationMW{comp: c},
	}
}

// validatePageTablePageSize checks if the provided page table's page size is
// consistent with the MMU's log2PageSize configuration.
func validatePageTablePageSize(pt vm.PageTable, log2PageSize uint64) {
	if pageTableInterface, ok := pt.(pageTable); ok {
		pageTableLog2PageSize := pageTableInterface.GetLog2PageSize()
		if pageTableLog2PageSize != log2PageSize {
			panic("page table page size does not match MMU page size")
		}
	}
}
