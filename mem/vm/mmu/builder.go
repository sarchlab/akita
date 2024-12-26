package mmu

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Builder can build MMU component
type Builder struct {
	simulation               simulation.Simulation
	freq                     timing.Freq
	log2PageSize             uint64
	pageTable                vm.PageTable
	migrationServiceProvider modeling.RemotePort
	maxNumReqInFlight        int
	pageWalkingLatency       int
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:              1 * timing.GHz,
		log2PageSize:      12,
		maxNumReqInFlight: 16,
	}
}

// WithSimulation sets the simulation to be used with the MMU
func (b Builder) WithSimulation(simulation simulation.Simulation) Builder {
	b.simulation = simulation
	return b
}

// WithFreq sets the frequency that the MMU to work at
func (b Builder) WithFreq(freq timing.Freq) Builder {
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
func (b Builder) WithMigrationServiceProvider(p modeling.RemotePort) Builder {
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
	mmu.TickingComponent = *modeling.NewTickingComponent(
		name, b.simulation.GetEngine(), b.freq, mmu)

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
	mmu.topPort = modeling.PortBuilder{}.
		WithComponent(mmu).
		WithSimulation(b.simulation).
		WithIncomingBufCap(4096).
		WithOutgoingBufCap(4096).
		Build(name + ".ToTop")
	mmu.AddPort("Top", mmu.topPort)

	mmu.migrationPort = modeling.PortBuilder{}.
		WithComponent(mmu).
		WithSimulation(b.simulation).
		WithIncomingBufCap(1).
		WithOutgoingBufCap(1).
		Build(name + ".MigrationPort")
	mmu.AddPort("Migration", mmu.migrationPort)
}
