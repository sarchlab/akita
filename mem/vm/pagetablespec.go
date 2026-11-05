package vm

import "github.com/sarchlab/akita/v5/sim/modeling"

// PageTableSpec declares a shared page table. Populate mappings after Initialize.
type PageTableSpec struct{ Log2PageSize uint64 }

// BuildResource constructs and registers a page table during blueprint initialization.
func (s PageTableSpec) BuildResource(sim modeling.Simulation, name string, _ modeling.None) *PageTable {
	if s.Log2PageSize >= 64 {
		panic("PageTableSpec: Log2PageSize must be less than 64")
	}
	return MakePageTableBuilder().WithSimulation(sim).WithLog2PageSize(s.Log2PageSize).Build(name)
}
