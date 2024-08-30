// Package gmmu provides the implementation of the Graphics Memory Management Unit (GMMU).
// It includes structures and methods for handling memory translation, page migration,
// and other related operations within the virtual memory system.
package gmmu

import (
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/sim"
)

// A Builder can build GMMU component
type Builder struct {
	engine             sim.Engine
	freq               sim.Freq
	log2PageSize       uint64
	pageTable          vm.PageTable
	maxNumReqInFlight  int
	pageWalkingLatency int
	deviceID           uint64
	lowModule          sim.Port
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:              1 * sim.GHz,
		log2PageSize:      12,
		maxNumReqInFlight: 16,
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

func (b Builder) configureInternalStates(gmmu *Comp) {
	gmmu.maxRequestsInFlight = b.maxNumReqInFlight
	gmmu.latency = b.pageWalkingLatency
	gmmu.PageAccessedByDeviceID = make(map[uint64][]uint64)
	gmmu.deviceID = b.deviceID
	gmmu.LowModule = b.lowModule
}

func (b Builder) createPageTable(gmmu *Comp) {
	if b.pageTable != nil {
		gmmu.pageTable = b.pageTable
	} else {
		gmmu.pageTable = vm.NewPageTable(b.log2PageSize)
	}
}

func (b Builder) createPorts(name string, gmmu *Comp) {
	gmmu.topPort = sim.NewLimitNumMsgPort(gmmu, 4096, name+".ToTop")
	gmmu.AddPort("Top", gmmu.topPort)
	gmmu.bottomPort = sim.NewLimitNumMsgPort(gmmu, 4096, name+".BottomPort")
	gmmu.AddPort("Bottom", gmmu.bottomPort)

	gmmu.topSender = sim.NewBufferedSender(
		gmmu.topPort, sim.NewBuffer(name+".TopSenderBuffer", 4096))
	gmmu.bottomSender = sim.NewBufferedSender(
		gmmu.bottomPort, sim.NewBuffer(name+".BottomSenderBuffer", 4096))

	gmmu.remoteMemReqs = make(map[uint64]transaction)
}

func (b Builder) Build(name string) *Comp {
	gmmu := new(Comp)
	gmmu.TickingComponent = *sim.NewTickingComponent(
		name, b.engine, b.freq, gmmu)

	b.createPorts(name, gmmu)
	b.createPageTable(gmmu)
	b.configureInternalStates(gmmu)

	return gmmu
}
