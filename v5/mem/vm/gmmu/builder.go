package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides the default configuration for GMMU components.
var DefaultSpec = Spec{
	Freq:                1 * sim.GHz,
	Log2PageSize:        12,
	MaxRequestsInFlight: 16,
}

// A Builder can build GMMU component
type Builder struct {
	engine             sim.Engine
	spec               Spec
	pageTable          vm.PageTable
	pageWalkingLatency int
	topPort            sim.Port
	bottomPort         sim.Port
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine to be used with the GMMU
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency at which the GMMU works.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithLog2PageSize sets the page size that the GMMU supports.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.spec.Log2PageSize = log2PageSize
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
	b.spec.MaxRequestsInFlight = maxNumReqInFlight
	return b
}

// WithPageWalkingLatency sets the latency of page walking
func (b Builder) WithPageWalkingLatency(pageWalkingLatency int) Builder {
	b.pageWalkingLatency = pageWalkingLatency
	return b
}

// WithDeviceID sets the device ID of the GMMU
func (b Builder) WithDeviceID(deviceID uint64) Builder {
	b.spec.DeviceID = deviceID
	return b
}

// WithLowModule sets the low module of the GMMU
func (b Builder) WithLowModule(p sim.RemotePort) Builder {
	b.spec.LowModule = p
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

// Build returns a new GMMU
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	spec := b.spec
	spec.Latency = b.pageWalkingLatency

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)

	initialState := State{
		RemoteMemReqs: make(map[string]transactionState),
	}
	modelComp.SetState(initialState)

	pt := b.pageTable
	if pt == nil {
		pt = vm.NewPageTable(b.spec.Log2PageSize)
	}

	wMW := &walkMW{
		comp:      modelComp,
		pageTable: pt,
	}
	modelComp.AddMiddleware(wMW)

	rMW := &respondMW{
		comp: modelComp,
	}
	modelComp.AddMiddleware(rMW)

	b.createPorts(modelComp)

	return modelComp
}

func (b Builder) createPorts(
	modelComp *modeling.Component[Spec, State],
) {
	b.topPort.SetComponent(modelComp)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(modelComp)
	modelComp.AddPort("Bottom", b.bottomPort)
}
