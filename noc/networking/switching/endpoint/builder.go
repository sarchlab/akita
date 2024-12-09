package endpoint

import (
	"container/list"
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
)

// Builder can help building End Points.
type Builder struct {
	engine                   sim.Engine
	freq                     sim.Freq
	numInputChannels         int
	numOutputChannels        int
	flitByteSize             int
	encodingOverhead         float64
	flitAssemblingBufferSize int
	networkPortBufferSize    int
	devicePorts              []sim.Port
}

// MakeBuilder creates a new EndPointBuilder with default
// configurations.
func MakeBuilder() Builder {
	return Builder{
		flitByteSize:             32,
		flitAssemblingBufferSize: 64,
		networkPortBufferSize:    4,
		freq:                     1 * sim.GHz,
		numInputChannels:         1,
		numOutputChannels:        1,
		encodingOverhead:         0.25,
	}
}

// WithEngine sets the engine of the End Point to build.
func (b Builder) WithEngine(e sim.Engine) Builder {
	b.engine = e
	return b
}

// WithFreq sets the frequency of the End Point to built.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithNumInputChannels sets the number of input channels of the End Point
// to build.
func (b Builder) WithNumInputChannels(num int) Builder {
	b.numInputChannels = num
	return b
}

// WithNumOutputChannels sets the number of output channels of the End Point
// to build.
func (b Builder) WithNumOutputChannels(num int) Builder {
	b.numOutputChannels = num
	return b
}

// WithFlitByteSize sets the flit byte size that the End Point supports.
func (b Builder) WithFlitByteSize(n int) Builder {
	b.flitByteSize = n
	return b
}

// WithEncodingOverhead sets the encoding overhead.
func (b Builder) WithEncodingOverhead(o float64) Builder {
	b.encodingOverhead = o
	return b
}

// WithNetworkPortBufferSize sets the network port buffer size of the end point.
func (b Builder) WithNetworkPortBufferSize(n int) Builder {
	b.networkPortBufferSize = n
	return b
}

// WithDevicePorts sets a list of ports that communicate directly through the
// End Point.
func (b Builder) WithDevicePorts(ports []sim.Port) Builder {
	b.devicePorts = ports
	return b
}

// Build creates a new End Point.
func (b Builder) Build(name string) *Comp {
	b.engineMustBeGiven()
	b.freqMustBeGiven()
	b.flitByteSizeMustBeGiven()

	ep := &Comp{}
	ep.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, ep)
	ep.flitByteSize = b.flitByteSize

	ep.numInputChannels = b.numInputChannels
	ep.numOutputChannels = b.numOutputChannels

	ep.assemblingMsgs = list.New()
	ep.assemblingMsgTable = make(map[string]*list.Element)

	ep.encodingOverhead = b.encodingOverhead

	ep.NetworkPort = sim.NewPort(
		ep, b.networkPortBufferSize, b.networkPortBufferSize,
		fmt.Sprintf("%s.NetworkPort", ep.Name()))

	for _, dp := range b.devicePorts {
		ep.PlugIn(dp)
	}

	middleware := &middleware{Comp: ep}
	ep.AddMiddleware(middleware)

	return ep
}

func (b Builder) engineMustBeGiven() {
	if b.engine == nil {
		panic("engine is not given")
	}
}

func (b Builder) freqMustBeGiven() {
	if b.freq == 0 {
		panic("freq must be given")
	}
}

func (b Builder) flitByteSizeMustBeGiven() {
	if b.flitByteSize == 0 {
		panic("flit byte size must be given")
	}
}
