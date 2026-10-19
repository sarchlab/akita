package vm

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// PageTableBuilder builds PageTable resources. WithSimulation is required; Build registers the resource.
type PageTableBuilder struct {
	log2PageSize uint64
	simulation   modeling.Simulation
}

// MakePageTableBuilder returns a PageTableBuilder with a default 12-bit (4 KB)
// page size.
func MakePageTableBuilder() PageTableBuilder {
	return PageTableBuilder{
		log2PageSize: 12,
	}
}

// WithLog2PageSize sets the log2 of the page size in bytes.
func (b PageTableBuilder) WithLog2PageSize(log2PageSize uint64) PageTableBuilder {
	b.log2PageSize = log2PageSize
	return b
}

// WithSimulation wires the builder to a simulation so the built page table
// registers itself as a resource.
func (b PageTableBuilder) WithSimulation(sim modeling.Simulation) PageTableBuilder {
	b.simulation = sim
	return b
}

// Build creates and registers the resource. WithSimulation is required.
func (b PageTableBuilder) Build(name string) *PageTable {
	if b.simulation == nil {
		panic("PageTable: WithSimulation is required")
	}
	pt := NewPageTable(b.log2PageSize)
	pt.name = name

	b.simulation.RegisterResource(pt)

	return pt
}
