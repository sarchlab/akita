package addresstranslator

import (
	"github.com/sarchlab/akita/v4/mem"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A Builder can create address translators
type Builder struct {
	simulation          simulation.Simulation
	freq                timing.Freq
	translationProvider modeling.RemotePort
	ctrlPort            modeling.Port
	addressToPortMapper mem.AddressToPortMapper
	numReqPerCycle      int
	log2PageSize        uint64
	deviceID            uint64
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:           1 * timing.GHz,
		numReqPerCycle: 4,
		log2PageSize:   12,
		deviceID:       1,
	}
}

// WithSimulation sets the simulation to be used by the address translators
func (b Builder) WithSimulation(simulation simulation.Simulation) Builder {
	b.simulation = simulation
	return b
}

// WithFreq sets the frequency of the address translators
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithTranslationProvider sets the port that can provide the translation
// service. The port must be a port on a TLB or an MMU.
func (b Builder) WithTranslationProvider(p modeling.RemotePort) Builder {
	b.translationProvider = p
	return b
}

// WithAddressToPortMapper sets the low modules finder that can tell the address
// translators where to send the memory access request to.
func (b Builder) WithAddressToPortMapper(f mem.AddressToPortMapper) Builder {
	b.addressToPortMapper = f
	return b
}

// WithNumReqPerCycle sets the number of request the address translators can
// process in each cycle.
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	return b
}

// WithDeviceID sets the GPU ID that the address translator belongs to
func (b Builder) WithDeviceID(n uint64) Builder {
	b.deviceID = n
	return b
}

// WithCtrlPort sets the port of the component that can send ctrl reqs to AT
func (b Builder) WithCtrlPort(p modeling.Port) Builder {
	b.ctrlPort = p
	return b
}

// Build returns a new AddressTranslator
func (b Builder) Build(name string) *Comp {
	t := &Comp{}
	t.TickingComponent = modeling.NewTickingComponent(
		name, b.simulation.GetEngine(), b.freq, t)

	b.createPorts(name, t)

	t.translationProvider = b.translationProvider
	t.addressToPortMapper = b.addressToPortMapper
	t.numReqPerCycle = b.numReqPerCycle
	t.log2PageSize = b.log2PageSize
	t.deviceID = b.deviceID

	middleware := &middleware{Comp: t}
	t.AddMiddleware(middleware)

	return t
}

func (b Builder) createPorts(name string, t *Comp) {
	t.topPort = modeling.PortBuilder{}.
		WithComponent(t).
		WithSimulation(b.simulation).
		WithIncomingBufCap(b.numReqPerCycle).
		WithOutgoingBufCap(b.numReqPerCycle).
		Build(name + ".TopPort")
	t.AddPort("Top", t.topPort)

	t.bottomPort = modeling.PortBuilder{}.
		WithComponent(t).
		WithSimulation(b.simulation).
		WithIncomingBufCap(b.numReqPerCycle).
		WithOutgoingBufCap(b.numReqPerCycle).
		Build(name + ".BottomPort")
	t.AddPort("Bottom", t.bottomPort)

	t.translationPort = modeling.PortBuilder{}.
		WithComponent(t).
		WithSimulation(b.simulation).
		WithIncomingBufCap(b.numReqPerCycle).
		WithOutgoingBufCap(b.numReqPerCycle).
		Build(name + ".TranslationPort")
	t.AddPort("Translation", t.translationPort)

	t.ctrlPort = modeling.PortBuilder{}.
		WithComponent(t).
		WithSimulation(b.simulation).
		WithIncomingBufCap(1).
		WithOutgoingBufCap(1).
		Build(name + ".CtrlPort")
	t.AddPort("Control", t.ctrlPort)
}
