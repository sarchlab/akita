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
	mmu.PageAccessedByDeviceID = make(map[uint64][]uint64)
}

func (b Builder) createPageTable(mmu *Comp) {
	if b.pageTable != nil {
		mmu.pageTable = b.pageTable
	} else {
		mmu.pageTable = vm.NewPageTable(b.log2PageSize)
	}
}

func (b Builder) createPorts(name string, mmu *Comp) {
	mmu.topPort = sim.NewPort(mmu, 4096, 4096, name+".ToTop")
	mmu.AddPort("Top", mmu.topPort)
	mmu.migrationPort = sim.NewPort(mmu, 1, 1, name+".MigrationPort")
	mmu.AddPort("Migration", mmu.migrationPort)
}
