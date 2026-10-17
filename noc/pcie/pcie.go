// Package pcie provides a Connector and establishes a PCIe connection.
package pcie

import (
	"math"

	"github.com/sarchlab/akita/v5/noc/networkconnector"

	"github.com/sarchlab/akita/v5/sim/timing"

	// Connector can connect devices into a PCIe network.
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type Connector struct {
	freq          timing.Freq
	bandwidth     uint64
	flitByteSize  int
	switchLatency int
	connector     *networkconnector.Connector
}

// Builder configures an independent connector; With methods do not assemble a network.
type Builder struct {
	simulation    timing.Simulation
	freq          timing.Freq
	bandwidth     uint64
	switchLatency int
}

func MakeBuilder() Builder { return Builder{freq: timing.GHz, switchLatency: 140}.WithVersion(4, 16) }

func (b Builder) WithSimulation(sim timing.Simulation) Builder { b.simulation = sim; return b }

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq

	return b
}

func (b Builder) WithBandwidth(bytePerSecond uint64) Builder { b.bandwidth = bytePerSecond; return b }

func (b Builder) WithVersion(version int, width int) Builder {
	linkBandwidthTable := map[int]uint64{
		1: 2 * (1 << 30),
		2: 4 * (1 << 30),
		3: 8 * (1 << 30),
		4: 16 * (1 << 30),
		5: 32 * (1 << 30),
	}

	linkBandwidth := linkBandwidthTable[version]
	totalBandwidth := linkBandwidth * uint64(width) / 8

	return b.WithBandwidth(totalBandwidth)
}

func (b Builder) WithSwitchLatency(numCycles int) Builder {
	b.switchLatency = numCycles
	return b
}

// Build computes derived parameters and begins a new network.
func (b Builder) Build(name string) *Connector {
	size := int(math.Round(float64(b.bandwidth) / float64(b.freq)))
	if b.freq <= 0 || size <= 0 {
		panic("pcie: frequency and derived flit size must be positive")
	}
	return &Connector{freq: b.freq, bandwidth: b.bandwidth, flitByteSize: size, switchLatency: b.switchLatency,
		connector: networkconnector.MakeBuilder().WithSimulation(b.simulation).
			WithFreq(b.freq).WithFlitSize(size).Build(name)}
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int) {
	switchID = c.connector.AddSwitch()

	c.PlugInDevice(switchID, cpuPorts)

	return switchID
}

// AddSwitch adds a new switch connecting from an existing switch.
func (c *Connector) AddSwitch(baseSwitchID int) (switchID int) {
	switchID = c.connector.AddSwitch()

	c.connector.ConnectSwitches(baseSwitchID, switchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})

	return switchID
}

// PlugInDevice connects a series of ports to a switch.
func (c *Connector) PlugInDevice(baseSwitchID int, devicePorts []messaging.Port) {
	c.connector.ConnectDevice(baseSwitchID, devicePorts,
		networkconnector.DeviceToSwitchLinkParameter{
			DeviceEndParam: networkconnector.LinkEndDeviceParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			SwitchEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})
}
