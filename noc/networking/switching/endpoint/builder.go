package endpoint

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// DefaultSpec provides the default configuration for endpoint components.
var DefaultSpec = Spec{
	Freq:              1 * timing.GHz,
	NumInputChannels:  1,
	NumOutputChannels: 1,
	FlitByteSize:      32,
	EncodingOverhead:  0.25,
}

// Builder can help building End Points.
type Builder struct {
	engine                   timing.EventScheduler
	registrar                modeling.Registrar
	spec                     Spec
	flitAssemblingBufferSize int
	networkPortBufferSize    int
	devicePorts              []messaging.Port
	networkPort              messaging.Port
}

// MakeBuilder creates a new EndPointBuilder with default
// configurations.
func MakeBuilder() Builder {
	return Builder{
		spec:                     DefaultSpec,
		flitAssemblingBufferSize: 64,
		networkPortBufferSize:    4,
	}
}

// WithEngine sets the engine of the End Point to build.
func (b Builder) WithEngine(e timing.EventScheduler) Builder {
	b.engine = e
	return b
}

// WithSimulation wires the builder to a simulation. It sources the engine from
// the simulation and registers the built component with it.
func (b Builder) WithSimulation(sim modeling.Registrar) Builder {
	b.registrar = sim
	b.engine = sim.GetEngine()
	return b
}

// WithFreq sets the frequency of the End Point to built.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNumInputChannels sets the number of input channels of the End Point
// to build.
func (b Builder) WithNumInputChannels(num int) Builder {
	b.spec.NumInputChannels = num
	return b
}

// WithNumOutputChannels sets the number of output channels of the End Point
// to build.
func (b Builder) WithNumOutputChannels(num int) Builder {
	b.spec.NumOutputChannels = num
	return b
}

// WithFlitByteSize sets the flit byte size that the End Point supports.
func (b Builder) WithFlitByteSize(n int) Builder {
	b.spec.FlitByteSize = n
	return b
}

// WithEncodingOverhead sets the encoding overhead.
func (b Builder) WithEncodingOverhead(o float64) Builder {
	b.spec.EncodingOverhead = o
	return b
}

// WithNetworkPortBufferSize sets the network port buffer size of the end point.
func (b Builder) WithNetworkPortBufferSize(n int) Builder {
	b.networkPortBufferSize = n
	return b
}

// WithDevicePorts sets a list of ports that communicate directly through the
// End Point.
func (b Builder) WithDevicePorts(ports []messaging.Port) Builder {
	b.devicePorts = ports
	return b
}

// WithNetworkPort sets the network port of the End Point.
func (b Builder) WithNetworkPort(port messaging.Port) Builder {
	b.networkPort = port
	return b
}

// Build creates a new End Point.
func (b Builder) Build(name string) *Comp {
	b.engineMustBeGiven()
	b.freqMustBeGiven()
	b.flitByteSizeMustBeGiven()

	spec := b.spec

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)

	outMW := &outgoingMW{
		comp:        modelComp,
		networkPort: b.networkPort,
	}

	inMW := &incomingMW{
		comp:        modelComp,
		networkPort: b.networkPort,
	}

	ep := &Comp{
		Component: modelComp,
	}

	ep.AddMiddleware(outMW)
	ep.AddMiddleware(inMW)

	b.networkPort.SetComponent(ep)

	for _, dp := range b.devicePorts {
		ep.PlugIn(dp)
	}

	if b.registrar != nil {
		b.registrar.RegisterComponent(ep)
	}

	return ep
}

func (b Builder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine is not given")
	}
}

func (b Builder) freqMustBeGiven() {
	if b.spec.Freq == 0 {
		panic("freq must be given")
	}
}

func (b Builder) flitByteSizeMustBeGiven() {
	if b.spec.FlitByteSize == 0 {
		panic("flit byte size must be given")
	}
}
