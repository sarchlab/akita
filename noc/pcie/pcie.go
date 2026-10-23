// Package pcie provides a Connector and establishes a PCIe connection.
package pcie

import (
	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

type Connector struct {
	inputWidth  int
	outputWidth int

	switchLatency int
	connector     *networkconnector.Connector
}

// Builder configures an independent connector; With methods do not assemble a network.
type Builder struct {
	flitByteSize int
	inputWidth   int
	outputWidth  int

	simulation modeling.Simulation
	freq       timing.Freq

	switchLatency int
}

// MakeBuilder returns the default direct configuration.
func MakeBuilder() Builder {
	return Builder{
		inputWidth:  1,
		outputWidth: 1,

		freq:          timing.GHz,
		flitByteSize:  34,
		switchLatency: 140,
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

// WithInputWidth sets flits per input per cycle and the calculated incoming,
// routing, and forwarding queue capacities.
func (b Builder) WithInputWidth(n int) Builder { b.inputWidth = n; return b }

// WithOutputWidth sets flits per output per cycle and the calculated send and
// outgoing queue capacities.
func (b Builder) WithOutputWidth(n int) Builder { b.outputWidth = n; return b }

// Build begins a network using the configured direct parameters.
func (b Builder) Build(name string) *Connector {
	if b.inputWidth <= 0 || b.outputWidth <= 0 {
		panic("pcie: widths must be positive")
	}
	if b.switchLatency < 0 {
		panic("pcie: switch latency cannot be negative")
	}

	return &Connector{
		inputWidth:  b.inputWidth,
		outputWidth: b.outputWidth,

		switchLatency: b.switchLatency,
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
			LeftEndParam:  c.switchEnd(c.switchLatency),
			RightEndParam: c.switchEnd(c.switchLatency),
		})

	return switchID
}

// ConnectDevice connects a series of ports to a switch.
func (c *Connector) ConnectDevice(baseSwitchID int, devicePorts []messaging.Port) {
	c.connector.ConnectDevice(baseSwitchID, devicePorts,
		networkconnector.DeviceConfig{
			DeviceEndParam: c.deviceEnd(),
			SwitchEndParam: c.switchEnd(c.switchLatency),
		})
}

func (c *Connector) switchEnd(latency int) networkconnector.LinkEndSwitchParameter {
	return networkconnector.LinkEndSwitchParameter{
		NumInputChannel:  c.inputWidth,
		NumOutputChannel: c.outputWidth,
		Latency:          latency,
	}
}

func (c *Connector) deviceEnd() networkconnector.LinkEndDeviceParameter {
	return networkconnector.LinkEndDeviceParameter{
		NumInputChannel:  c.inputWidth,
		NumOutputChannel: c.outputWidth,
	}
}
