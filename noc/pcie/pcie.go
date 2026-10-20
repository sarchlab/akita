// Package pcie provides a Connector and establishes a PCIe connection.
package pcie

import (
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

type Connector struct {
	inputWidth       int
	outputWidth      int
	incomingCapacity int
	outgoingCapacity int
	routeCapacity    int
	forwardCapacity  int
	sendCapacity     int

	freq timing.Freq

	flitByteSize  int
	switchLatency int
	connector     *networkconnector.Connector
}

// Builder configures an independent connector; With methods do not assemble a network.
type Builder struct {
	flitByteSize     int
	inputWidth       int
	outputWidth      int
	incomingCapacity int
	outgoingCapacity int
	routeCapacity    int
	forwardCapacity  int
	sendCapacity     int

	simulation modeling.Simulation
	freq       timing.Freq

	switchLatency int
}

// MakeBuilder returns the default direct configuration.
func MakeBuilder() Builder {
	return Builder{
		inputWidth:       1,
		outputWidth:      1,
		incomingCapacity: 16,
		outgoingCapacity: 16,
		routeCapacity:    1,
		forwardCapacity:  1,
		sendCapacity:     1,
		freq:             timing.GHz,
		flitByteSize:     34,
		switchLatency:    140,
	}
}

func (b Builder) WithSimulation(sim modeling.Simulation) Builder { b.simulation = sim; return b }

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq

	return b
}

func (b Builder) WithSwitchLatency(numCycles int) Builder {
	b.switchLatency = numCycles
	return b
}

// WithFlitSize sets bytes per flit independently of frequency.
func (b Builder) WithFlitSize(size int) Builder { b.flitByteSize = size; return b }

// WithInputWidth sets the flits per cycle.
func (b Builder) WithInputWidth(n int) Builder { b.inputWidth = n; return b }

// WithOutputWidth sets the flits per cycle.
func (b Builder) WithOutputWidth(n int) Builder { b.outputWidth = n; return b }

// WithIncomingBufferCapacity sets the queue capacity in flits.
func (b Builder) WithIncomingBufferCapacity(n int) Builder { b.incomingCapacity = n; return b }

// WithOutgoingBufferCapacity sets the queue capacity in flits.
func (b Builder) WithOutgoingBufferCapacity(n int) Builder { b.outgoingCapacity = n; return b }

// WithRouteBufferCapacity sets the queue capacity in flits.
func (b Builder) WithRouteBufferCapacity(n int) Builder { b.routeCapacity = n; return b }

// WithForwardBufferCapacity sets the queue capacity in flits.
func (b Builder) WithForwardBufferCapacity(n int) Builder { b.forwardCapacity = n; return b }

// WithSendBufferCapacity sets the queue capacity in flits.
func (b Builder) WithSendBufferCapacity(n int) Builder { b.sendCapacity = n; return b }

// Build begins a network using the configured direct parameters.
func (b Builder) Build(name string) *Connector {
	if b.inputWidth <= 0 ||
		b.outputWidth <= 0 ||
		b.incomingCapacity <= 0 ||
		b.outgoingCapacity <= 0 ||
		b.routeCapacity <= 0 ||
		b.forwardCapacity <= 0 ||
		b.sendCapacity <= 0 {
		panic("pcie: widths and buffer capacities must be positive")
	}
	if b.switchLatency < 0 {
		panic("pcie: switch latency cannot be negative")
	}

	return &Connector{
		inputWidth:       b.inputWidth,
		outputWidth:      b.outputWidth,
		incomingCapacity: b.incomingCapacity,
		outgoingCapacity: b.outgoingCapacity,
		routeCapacity:    b.routeCapacity,
		forwardCapacity:  b.forwardCapacity,
		sendCapacity:     b.sendCapacity,
		freq:             b.freq,
		flitByteSize:     b.flitByteSize,
		switchLatency:    b.switchLatency,
		connector: networkconnector.MakeBuilder().WithSimulation(b.simulation).
			WithFreq(b.freq).WithFlitSize(b.flitByteSize).Build(name)}
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int) {
	switchID = c.connector.AddSwitch(networkconnector.SwitchConfig{})

	c.ConnectDevice(switchID, cpuPorts)

	return switchID
}

// AddSwitch adds a new switch connecting from an existing switch.
func (c *Connector) AddSwitch(baseSwitchID int) (switchID int) {
	switchID = c.connector.AddSwitch(networkconnector.SwitchConfig{})

	c.connector.ConnectSwitches(baseSwitchID, switchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				RouteBufferCapacity:   c.routeCapacity,
				ForwardBufferCapacity: c.forwardCapacity,
				SendBufferCapacity:    c.sendCapacity,
				IncomingBufSize:       c.incomingCapacity,
				OutgoingBufSize:       c.outgoingCapacity,
				Latency:               c.switchLatency,
				NumInputChannel:       c.inputWidth,
				NumOutputChannel:      c.outputWidth,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				RouteBufferCapacity:   c.routeCapacity,
				ForwardBufferCapacity: c.forwardCapacity,
				SendBufferCapacity:    c.sendCapacity,
				IncomingBufSize:       c.incomingCapacity,
				OutgoingBufSize:       c.outgoingCapacity,
				Latency:               c.switchLatency,
				NumInputChannel:       c.inputWidth,
				NumOutputChannel:      c.outputWidth,
			},
		})

	return switchID
}

// ConnectDevice connects a series of ports to a switch.
func (c *Connector) ConnectDevice(baseSwitchID int, devicePorts []messaging.Port) {
	c.connector.ConnectDevice(baseSwitchID, devicePorts,
		networkconnector.DeviceConfig{
			DeviceEndParam: networkconnector.LinkEndDeviceParameter{
				IncomingBufSize:  c.incomingCapacity,
				OutgoingBufSize:  c.outgoingCapacity,
				NumInputChannel:  c.inputWidth,
				NumOutputChannel: c.outputWidth,
			},
			SwitchEndParam: networkconnector.LinkEndSwitchParameter{
				RouteBufferCapacity:   c.routeCapacity,
				ForwardBufferCapacity: c.forwardCapacity,
				SendBufferCapacity:    c.sendCapacity,
				IncomingBufSize:       c.incomingCapacity,
				OutgoingBufSize:       c.outgoingCapacity,
				Latency:               c.switchLatency,
				NumInputChannel:       c.inputWidth,
				NumOutputChannel:      c.outputWidth,
			},
		})
}
