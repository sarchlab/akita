package gmmu

import (
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/sim"
)

// A Builder can build GMMU component
type Builder struct {
	engine                   sim.Engine
	freq                     sim.Freq
	log2PageSize             uint64
	pageTable                vm.PageTable
	migrationServiceProvider sim.Port
	maxNumReqInFlight        int
	pageWalkingLatency       int
	deviceID                 uint64
	lowModule                sim.Port
	isRecording              bool
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:              1 * sim.GHz,
		log2PageSize:      12,
		maxNumReqInFlight: 16,
		isRecording:       false,
	}
}

// WithEngine sets the engine to be used with the GMMU
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the GMMU to work at
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2PageSize sets the page size that the gmmu support.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.log2PageSize = log2PageSize
	return b
}

// WithPageTable sets the page table that the GMMU uses.
func (b Builder) WithPageTable(pageTable vm.PageTable) Builder {
	b.pageTable = pageTable
	return b
}

// WithMigrationServiceProvider sets the destination port that can perform
// page migration.
func (b Builder) WithMigrationServiceProvider(p sim.Port) Builder {
	b.migrationServiceProvider = p
	return b
}

// WithMaxNumReqInFlight sets the number of requests can be concurrently
// processed by the GMMU.
func (b Builder) WithMaxNumReqInFlight(maxNumReqInFlight int) Builder {
	b.maxNumReqInFlight = maxNumReqInFlight
	return b
}

// WithPageWalkingLatency sets the latency of page walking
func (b Builder) WithPageWalkingLatency(pageWalkingLatency int) Builder {
	b.pageWalkingLatency = pageWalkingLatency
	return b
}

// WithDeviceID sets the device ID of the GMMU
func (b Builder) WithDeviceID(deviceID uint64) Builder {
	b.deviceID = deviceID
	return b
}

// WithLowModule sets the low module of the GMMU
func (b Builder) WithLowModule(p sim.Port) Builder {
	b.lowModule = p
	return b
}

// WithRecording sets whether the GMMU is recording
func (b Builder) WithRecording(isRecording bool) Builder {
	b.isRecording = isRecording

	return b
}

func (b Builder) configureInternalStates(gmmu *GMMU) {
	gmmu.MigrationServiceProvider = b.migrationServiceProvider
	gmmu.migrationQueueSize = 4096
	gmmu.maxRequestsInFlight = b.maxNumReqInFlight
	gmmu.latency = b.pageWalkingLatency
	gmmu.PageAccessedByDeviceID = make(map[uint64][]uint64)
	gmmu.deviceID = b.deviceID
	gmmu.LowModule = b.lowModule
	gmmu.isRecording = b.isRecording
}

func (b Builder) createPageTable(gmmu *GMMU) {
	if b.pageTable != nil {
		gmmu.pageTable = b.pageTable
	} else {
		gmmu.pageTable = vm.NewPageTable(b.log2PageSize)
	}
}

func (b Builder) createPorts(name string, gmmu *GMMU) {
	gmmu.topPort = sim.NewLimitNumMsgPort(gmmu, 4096, name+".ToTop")
	gmmu.AddPort("Top", gmmu.topPort)
	gmmu.migrationPort = sim.NewLimitNumMsgPort(gmmu, 1, name+".MigrationPort")
	gmmu.AddPort("Migration", gmmu.migrationPort)
	gmmu.bottomPort = sim.NewLimitNumMsgPort(gmmu, 4096, name+".BottomPort")
	gmmu.AddPort("Bottom", gmmu.bottomPort)

	gmmu.topSender = sim.NewBufferedSender(
		gmmu.topPort, sim.NewBuffer(name+".TopSenderBuffer", 4096))
	gmmu.bottomSender = sim.NewBufferedSender(
		gmmu.bottomPort, sim.NewBuffer(name+".BottomSenderBuffer", 4096))

	gmmu.remoteMemReqs = make(map[uint64]transaction)
}

func (b Builder) Build(name string) *GMMU {
	gmmu := new(GMMU)
	gmmu.TickingComponent = *sim.NewTickingComponent(
		name, b.engine, b.freq, gmmu)

	b.createPorts(name, gmmu)
	b.createPageTable(gmmu)
	b.configureInternalStates(gmmu)

	return gmmu
}
