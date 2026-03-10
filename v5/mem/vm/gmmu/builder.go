package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
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
	lowModule          sim.RemotePort
	topPort            sim.Port
	bottomPort         sim.Port
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

// WithFreq sets the frequency at which the GMMU works.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2PageSize sets the page size that the GMMU supports.
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
func (b Builder) WithLowModule(p sim.RemotePort) Builder {
	b.lowModule = p
	return b
}

// WithTopPort sets the top port of the GMMU
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port of the GMMU
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

func (b Builder) Build(name string) *GMMU {
	spec := Spec{
		DeviceID:            b.deviceID,
		Log2PageSize:        b.log2PageSize,
		Latency:             b.pageWalkingLatency,
		MaxRequestsInFlight: b.maxNumReqInFlight,
		LowModule:           b.lowModule,
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSpec(spec).
		Build(name)

	gmmu := &GMMU{
		Component:              modelComp,
		PageAccessedByDeviceID: make(map[uint64][]uint64),
		remoteMemReqs:          make(map[string]transaction),
	}

	b.createPageTable(gmmu)
	b.createPorts(name, gmmu)

	middleware := &gmmuMiddleware{GMMU: gmmu}
	gmmu.AddMiddleware(middleware)

	return gmmu
}

func (b Builder) createPageTable(gmmu *GMMU) {
	if b.pageTable != nil {
		gmmu.pageTable = b.pageTable
	} else {
		gmmu.pageTable = vm.NewPageTable(b.log2PageSize)
	}
}

func (b Builder) createPorts(name string, gmmu *GMMU) {
	gmmu.topPort = b.topPort
	gmmu.topPort.SetComponent(gmmu)
	gmmu.AddPort("Top", gmmu.topPort)
	gmmu.bottomPort = b.bottomPort
	gmmu.bottomPort.SetComponent(gmmu)
	gmmu.AddPort("Bottom", gmmu.bottomPort)
}
