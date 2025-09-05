package mmu

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
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

// Build returns a newly created MMU component
func (b Builder) Build(name string) *Comp {
	mmu := new(Comp)
	mmu.TickingComponent = *sim.NewTickingComponent(
		name, b.engine, b.freq, mmu)

	b.createPorts(name, mmu)
	b.createPageTable(mmu)
	b.configureInternalStates(mmu)

	middleware := &middleware{Comp: mmu}
	mmu.AddMiddleware(middleware)

	return mmu
}

func (b Builder) configureInternalStates(mmu *Comp) {
	mmu.MigrationServiceProvider = b.migrationServiceProvider
	mmu.migrationQueueSize = 4096
	mmu.maxRequestsInFlight = b.maxNumReqInFlight
	mmu.latency = b.pageWalkingLatency
	mmu.autoPageAllocation = b.autoPageAllocation
	mmu.log2PageSize = b.log2PageSize
	mmu.PageAccessedByDeviceID = make(map[uint64][]uint64)
	
	if mmu.autoPageAllocation {
		mmu.nextPhysicalPage = 0
	}
}

func (b Builder) createPageTable(mmu *Comp) {
	if b.pageTable != nil {
		// Check if the provided page table is compatible with the MMU's page size
		b.validatePageTablePageSize()
		mmu.pageTable = b.pageTable
	} else {
		mmu.pageTable = vm.NewPageTable(b.log2PageSize)
	}
}

// validatePageTablePageSize checks if the provided page table's page size
// is consistent with the MMU's log2PageSize configuration.
func (b Builder) validatePageTablePageSize() {
	// If the page table implements pageTable interface with GetLog2PageSize, validate the page size
	if pageTableInterface, ok := b.pageTable.(pageTable); ok {
		pageTableLog2PageSize := pageTableInterface.GetLog2PageSize()
		if pageTableLog2PageSize != b.log2PageSize {
			panic("page table page size does not match MMU page size")
		}
	}
	// For page tables that don't implement the local pageTable interface, we cannot validate
	// the page size so we assume the user has ensured compatibility
}

func (b Builder) createPorts(name string, mmu *Comp) {
	mmu.topPort = sim.NewPort(mmu, 4096, 4096, name+".ToTop")
	mmu.AddPort("Top", mmu.topPort)
	mmu.migrationPort = sim.NewPort(mmu, 1, 1, name+".MigrationPort")
	mmu.AddPort("Migration", mmu.migrationPort)
}
