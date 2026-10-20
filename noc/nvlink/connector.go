// Package nvlink provides a connector that can create a network that includes
// PCIe and NVLink network.
package nvlink

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

type deviceNode struct {
	deviceSwitchID int
	nvlinkSwitchID int
}

// Connector can connect devices into a network of PCIe and NVLink links.
type Connector struct {
	inputWidth       int
	outputWidth      int
	incomingCapacity int
	outgoingCapacity int
	routeCapacity    int
	forwardCapacity  int
	sendCapacity     int

	pcieSwitchLatency int

	nvlinkSwitchLatency int

	connector *networkconnector.Connector

	devices      []*deviceNode
	pcieSwitches map[int]bool
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

	pcieSwitchLatency int

	nvlinkSwitchLatency int
}

// MakeBuilder returns the default direct configuration.
func MakeBuilder() Builder {
	return Builder{
		inputWidth:          1,
		outputWidth:         1,
		incomingCapacity:    16,
		outgoingCapacity:    16,
		routeCapacity:       1,
		forwardCapacity:     1,
		sendCapacity:        1,
		freq:                timing.GHz,
		flitByteSize:        34,
		pcieSwitchLatency:   140,
		nvlinkSwitchLatency: 140,
	}
}

func (b Builder) WithSimulation(sim modeling.Simulation) Builder { b.simulation = sim; return b }

func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq

	return b
}

func (b Builder) WithPCIeSwitchLatency(numCycles int) Builder {
	b.pcieSwitchLatency = numCycles
	return b
}

func (b Builder) WithNVLinkSwitchLatency(numCycle int) Builder {
	b.nvlinkSwitchLatency = numCycle

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
		panic("nvlink: widths and buffer capacities must be positive")
	}
	if b.pcieSwitchLatency < 0 || b.nvlinkSwitchLatency < 0 {
		panic("nvlink: switch latency cannot be negative")
	}

	return &Connector{
		inputWidth:          b.inputWidth,
		outputWidth:         b.outputWidth,
		incomingCapacity:    b.incomingCapacity,
		outgoingCapacity:    b.outgoingCapacity,
		routeCapacity:       b.routeCapacity,
		forwardCapacity:     b.forwardCapacity,
		sendCapacity:        b.sendCapacity,
		pcieSwitchLatency:   b.pcieSwitchLatency,
		nvlinkSwitchLatency: b.nvlinkSwitchLatency, pcieSwitches: make(map[int]bool),
		connector: networkconnector.MakeBuilder().WithSimulation(b.simulation).
			WithFreq(b.freq).WithFlitSize(b.flitByteSize).Build(name)}
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int) {
	switchID = c.connector.AddSwitch(networkconnector.SwitchConfig{Name: "RootComplex"})

	c.ConnectDevice(switchID, cpuPorts)

	return switchID
}

// AddSwitch adds an unconnected PCIe switch. Connect it with ConnectSwitchesWithPCIeLink.
func (c *Connector) AddSwitch() (switchID int) {
	switchID = c.connector.AddSwitch(networkconnector.SwitchConfig{
		Name: fmt.Sprintf("PCIeSwitch[%d]", len(c.pcieSwitches)),
	})

	c.pcieSwitches[switchID] = true

	return switchID
}

// ConnectSwitchesWithPCIeLink connects two switches with a PCIe link.
func (c *Connector) ConnectSwitchesWithPCIeLink(switchAID, switchBID int) {
	c.connector.ConnectSwitches(switchAID, switchBID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam:  c.switchEnd(c.pcieSwitchLatency),
			RightEndParam: c.switchEnd(c.pcieSwitchLatency),
		})
}

// ConnectDevice connects a series of ports to a switch.
func (c *Connector) ConnectDevice(
	pcieSwitchID int,
	devicePorts []messaging.Port,
) (deviceID int) {
	deviceID = len(c.devices)

	deviceSwitchID := c.connector.AddSwitch(networkconnector.SwitchConfig{Name: fmt.Sprintf("DeviceSwitch[%d]", deviceID)})
	nvlinkSwitchID := c.connector.AddSwitch(networkconnector.SwitchConfig{Name: fmt.Sprintf("NVLinkSwitch[%d]", deviceID)})

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
			LeftEndParam:  c.switchEnd(1),
			RightEndParam: c.switchEnd(1),
		})
}

func (c *Connector) connectDeviceSwitchWithPCIeSwitch(
	pcieSwitchID int,
	deviceSwitchID int,
) {
	c.connector.ConnectSwitches(deviceSwitchID, pcieSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam:  c.switchEnd(1),
			RightEndParam: c.switchEnd(1),
		})
}

func (c *Connector) connectDeviceWithDeviceSwitch(
	deviceSwitchID int,
	devicePorts []messaging.Port,
) {
	c.connector.ConnectDevice(deviceSwitchID, devicePorts,
		networkconnector.DeviceConfig{
			DeviceEndParam: c.deviceEnd(),
			SwitchEndParam: c.switchEnd(c.pcieSwitchLatency),
		})
}

// ConnectDevicesWithNVLink establishes an NVLink Connection.
func (c *Connector) ConnectDevicesWithNVLink(
	deviceA, deviceB int,
) {
	deviceANode := c.devices[deviceA]
	deviceBNode := c.devices[deviceB]

	c.connector.ConnectSwitches(
		deviceANode.nvlinkSwitchID,
		deviceBNode.nvlinkSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam:  c.switchEnd(c.nvlinkSwitchLatency),
			RightEndParam: c.switchEnd(c.nvlinkSwitchLatency),
		})
}

func (c *Connector) switchEnd(latency int) networkconnector.LinkEndSwitchParameter {
	return networkconnector.LinkEndSwitchParameter{
		IncomingBufSize:       c.incomingCapacity,
		OutgoingBufSize:       c.outgoingCapacity,
		NumInputChannel:       c.inputWidth,
		NumOutputChannel:      c.outputWidth,
		RouteBufferCapacity:   c.routeCapacity,
		ForwardBufferCapacity: c.forwardCapacity,
		SendBufferCapacity:    c.sendCapacity,
		Latency:               latency,
	}
}

func (c *Connector) deviceEnd() networkconnector.LinkEndDeviceParameter {
	return networkconnector.LinkEndDeviceParameter{
		IncomingBufSize:  c.incomingCapacity,
		OutgoingBufSize:  c.outgoingCapacity,
		NumInputChannel:  c.inputWidth,
		NumOutputChannel: c.outputWidth,
	}
}
