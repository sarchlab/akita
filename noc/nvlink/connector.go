// Package nvlink provides a connector that can create a network that includes
// PCIe and NVLink network.
package nvlink

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	// A deviceNode represents a switch associated with the device and
	// and NVLink switch.
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type deviceNode struct {
	deviceSwitchID int
	nvlinkSwitchID int
}

// Connector can connect devices into a network of PCIe and NVLink links.
type Connector struct {
	freq         timing.Freq
	flitByteSize int

	pcieBandwidth     uint64
	pcieSwitchLatency int

	nvlinkBandwidth     uint64
	nvlinkSwitchLatency int

	connector *networkconnector.Connector

	devices      []*deviceNode
	pcieSwitches map[int]bool
}

// Builder configures an independent connector; With methods do not assemble a network.
type Builder struct {
	simulation          modeling.Simulation
	freq                timing.Freq
	pcieBandwidth       uint64
	pcieSwitchLatency   int
	nvlinkBandwidth     uint64
	nvlinkSwitchLatency int
}

func MakeBuilder() Builder {
	return Builder{freq: timing.GHz, pcieSwitchLatency: 140, nvlinkSwitchLatency: 140}.
		WithPCIeVersion(4, 16).WithNVLinkVersion(2)
}

func (b Builder) WithSimulation(sim modeling.Simulation) Builder { b.simulation = sim; return b }

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq

	return b
}

func (b Builder) WithPCIeBandwidth(bytePerSecond uint64) Builder {
	b.pcieBandwidth = bytePerSecond
	return b
}

func (b Builder) WithPCIeVersion(version int, width int) Builder {
	transferPerSecondTable := map[int]uint64{
		1: 2 * (1 << 30),
		2: 4 * (1 << 30),
		3: 8 * (1 << 30),
		4: 16 * (1 << 30),
		5: 32 * (1 << 30),
	}

	transferPerSecond := transferPerSecondTable[version]
	totalBandwidth := transferPerSecond * uint64(width) / 8

	return b.WithPCIeBandwidth(totalBandwidth)
}

func (b Builder) WithPCIeSwitchLatency(numCycles int) Builder {
	b.pcieSwitchLatency = numCycles
	return b
}

func (b Builder) WithNVLinkBandwidth(bytePerSecond uint64) Builder {
	b.nvlinkBandwidth = bytePerSecond
	return b
}

func (b Builder) WithNVLinkVersion(version int) Builder {
	switch version {
	case 1:
		b.nvlinkBandwidth = 2.5 * (1 << 30) * 8
	case 2:
		b.nvlinkBandwidth = 3.125 * (1 << 30) * 8
	case 3:
		b.nvlinkBandwidth = 6.25 * (1 << 30) * 8
	default:
		panic("NVLink version not supported.")
	}

	return b
}

func (b Builder) WithNVLinkSwitchLatency(numCycle int) Builder {
	b.nvlinkSwitchLatency = numCycle

	return b
}

// Build computes derived parameters and begins a new network.
func (b Builder) Build(name string) *Connector {
	size := int(math.Round(float64(b.pcieBandwidth) / float64(b.freq)))
	if b.freq <= 0 || size <= 0 || b.nvlinkBandwidth == 0 {
		panic("nvlink: frequency, bandwidth and derived flit size must be positive")
	}
	return &Connector{
		freq: b.freq, flitByteSize: size, pcieBandwidth: b.pcieBandwidth,
		pcieSwitchLatency: b.pcieSwitchLatency,
		nvlinkBandwidth:   b.nvlinkBandwidth, nvlinkSwitchLatency: b.nvlinkSwitchLatency, pcieSwitches: make(map[int]bool),
		connector: networkconnector.MakeBuilder().WithSimulation(b.simulation).
			WithFreq(b.freq).WithFlitSize(size).
			WithRouter(networkconnector.BandwidthFirstRouter{FlitSize: size}).Build(name)}
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int) {
	switchID = c.connector.AddSwitchWithName("RootComplex")

	c.PlugInDevice(switchID, cpuPorts)

	return switchID
}

// AddPCIeSwitch adds a new switch connecting from an existing switch.
func (c *Connector) AddPCIeSwitch() (switchID int) {
	switchID = c.connector.AddSwitchWithName(
		fmt.Sprintf("PCIeSwitch[%d]", len(c.pcieSwitches)))

	c.pcieSwitches[switchID] = true

	return switchID
}

// ConnectSwitchesWithPCIeLink connects two switches with a PCIe link.
func (c *Connector) ConnectSwitchesWithPCIeLink(switchAID, switchBID int) {
	c.connector.ConnectSwitches(switchAID, switchBID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

// PlugInDevice connects a series of ports to a switch.
func (c *Connector) PlugInDevice(
	pcieSwitchID int,
	devicePorts []messaging.Port,
) (deviceID int) {
	deviceID = len(c.devices)

	deviceSwitchID := c.connector.AddSwitchWithName(
		fmt.Sprintf("DeviceSwitch[%d]", deviceID))
	nvlinkSwitchID := c.connector.AddSwitchWithName(
		fmt.Sprintf("NVLinkSwitch[%d]", deviceID))

	c.connectDeviceSwitchWithNVLinkSwitch(deviceSwitchID, nvlinkSwitchID)
	c.connectDeviceSwitchWithPCIeSwitch(pcieSwitchID, deviceSwitchID)
	c.connectDeviceWithDeviceSwitch(deviceSwitchID, devicePorts)

	deviceNode := &deviceNode{
		deviceSwitchID: deviceSwitchID,
		nvlinkSwitchID: nvlinkSwitchID,
	}
	c.devices = append(c.devices, deviceNode)

	return deviceID
}

func (c *Connector) connectDeviceSwitchWithNVLinkSwitch(
	deviceSwitchID int,
	nvlinkSwitchID int,
) {
	c.connector.ConnectSwitches(deviceSwitchID, nvlinkSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

func (c *Connector) connectDeviceSwitchWithPCIeSwitch(
	pcieSwitchID int,
	deviceSwitchID int,
) {
	c.connector.ConnectSwitches(deviceSwitchID, pcieSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

func (c *Connector) connectDeviceWithDeviceSwitch(
	deviceSwitchID int,
	devicePorts []messaging.Port,
) {
	c.connector.ConnectDevice(deviceSwitchID, devicePorts,
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
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})
}

// ConnectDevicesWithNVLink establishes an NVLink Connection.
func (c *Connector) ConnectDevicesWithNVLink(
	deviceA, deviceB int,
	numLink int,
) {
	deviceANode := c.devices[deviceA]
	deviceBNode := c.devices[deviceB]

	freq := math.Round(float64(c.nvlinkBandwidth) / float64(c.flitByteSize))

	c.connector.ConnectSwitches(
		deviceANode.nvlinkSwitchID,
		deviceBNode.nvlinkSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.nvlinkSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.nvlinkSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     timing.Freq(freq),
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: numLink,
			},
		})
}
