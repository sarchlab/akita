package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// A Builder can build MMU component
type Builder struct {
	engine                   sim.Engine
	freq                     sim.Freq
	log2PageSize             uint64
	pageTable                vm.PageTable
	migrationServiceProvider sim.RemotePort
	maxNumReqInFlight        int
	pageWalkingLatency       int
	autoPageAllocation       bool
	topPort                  sim.Port
	migrationPort            sim.Port
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:              1 * sim.GHz,
		log2PageSize:      12,
		maxNumReqInFlight: 16,
	}
}

// WithEngine sets the engine to be used with the MMU
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the MMU to work at
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2PageSize sets the page size that the mmu support.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.log2PageSize = log2PageSize
	return b
}

// WithPageTable sets the page table that the MMU uses.
func (b Builder) WithPageTable(pageTable vm.PageTable) Builder {
	b.pageTable = pageTable
	return b
}

// WithMigrationServiceProvider sets the destination port that can perform
// page migration.
func (b Builder) WithMigrationServiceProvider(p sim.RemotePort) Builder {
	b.migrationServiceProvider = p
	return b
}

// WithMaxNumReqInFlight sets the number of requests can be concurrently
// processed by the MMU.
func (b Builder) WithMaxNumReqInFlight(n int) Builder {
	b.maxNumReqInFlight = n
	return b
}

// WithPageWalkingLatency sets the number of cycles required for walking a page
// table.
func (b Builder) WithPageWalkingLatency(n int) Builder {
	b.pageWalkingLatency = n
	return b
}

// WithAutoPageAllocation enables or disables automatic page allocation.
// When enabled, the MMU will automatically create page table entries for
// virtual addresses that don't exist, instead of panicking.
func (b Builder) WithAutoPageAllocation(enabled bool) Builder {
	b.autoPageAllocation = enabled
	return b
}

// WithTopPort sets the top port of the MMU
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithMigrationPort sets the migration port of the MMU
func (b Builder) WithMigrationPort(port sim.Port) Builder {
	b.migrationPort = port
	return b
}

// Build returns a newly created MMU component
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	spec := Spec{
		Latency:                  b.pageWalkingLatency,
		MaxRequestsInFlight:      b.maxNumReqInFlight,
		MigrationQueueSize:       4096,
		AutoPageAllocation:       b.autoPageAllocation,
		Log2PageSize:             b.log2PageSize,
		MigrationServiceProvider: b.migrationServiceProvider,
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	b.createPorts(name, modelComp)

	pt := b.createPageTable()

	mw := &middleware{comp: modelComp, pageTable: pt}
	modelComp.AddMiddleware(mw)

	return modelComp
}

func (b Builder) createPageTable() vm.PageTable {
	if b.pageTable != nil {
		b.validatePageTablePageSize()
		return b.pageTable
	}

	return vm.NewPageTable(b.log2PageSize)
}

// validatePageTablePageSize checks if the provided page table's page size
// is consistent with the MMU's log2PageSize configuration.
func (b Builder) validatePageTablePageSize() {
	if pageTableInterface, ok := b.pageTable.(pageTable); ok {
		pageTableLog2PageSize := pageTableInterface.GetLog2PageSize()
		if pageTableLog2PageSize != b.log2PageSize {
			panic("page table page size does not match MMU page size")
		}
	}
}

func (b Builder) createPorts(name string, mmu *modeling.Component[Spec, State]) {
	b.topPort.SetComponent(mmu)
	mmu.AddPort("Top", b.topPort)
	b.migrationPort.SetComponent(mmu)
	mmu.AddPort("Migration", b.migrationPort)
}
