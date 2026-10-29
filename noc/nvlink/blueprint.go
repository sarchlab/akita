package nvlink

import (
	"fmt"
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Device attaches ports to a PCIe switch. Switch zero denotes the root complex;
// switches 1..NumSwitches are added in that order. Device indices are slice indices.
type Device struct {
	Switch int                    `json:"switch"`
	Ports  []messaging.RemotePort `json:"ports"`
}

// Link names two switches (PCIeLinks) or two devices (NVLinks).
type Link struct {
	A int `json:"a"`
	B int `json:"b"`
}

// Spec declares a PCIe/NVLink network. Lists preserve assembly and binding order.
type Spec struct {
	Freq                timing.Freq
	FlitSize            int
	PCIeSwitchLatency   int
	NVLinkSwitchLatency int
	InputWidth          int
	OutputWidth         int
	NumSwitches         int
	RootPorts           []messaging.RemotePort
	PCIeLinks           []Link
	Devices             []Device
	NVLinks             []Link
}

// DefaultSpec returns settings matching MakeBuilder.
func DefaultSpec() Spec {
	return Spec{Freq: timing.GHz, FlitSize: 34, PCIeSwitchLatency: 140, NVLinkSwitchLatency: 140,
		InputWidth: 1, OutputWidth: 1}
}

// Validate checks parameters and indices without creating runtime objects.
func (s Spec) Validate() error {
	if s.Freq <= 0 || s.FlitSize <= 0 || s.InputWidth <= 0 || s.OutputWidth <= 0 || s.PCIeSwitchLatency < 0 ||
		s.NVLinkSwitchLatency < 0 || s.NumSwitches < 0 {
		return fmt.Errorf("NVLink parameters: positive frequency, flit size and widths; " +
			"nonnegative latencies and switch count required")
	}
	for i, d := range s.Devices {
		if d.Switch < 0 || d.Switch > s.NumSwitches {
			return fmt.Errorf("Devices[%d].Switch: unknown switch", i)
		}
	}
	for i, l := range s.PCIeLinks {
		if l.A < 0 || l.B < 0 || l.A > s.NumSwitches || l.B > s.NumSwitches || l.A == l.B {
			return fmt.Errorf("PCIeLinks[%d]: invalid switch pair", i)
		}
	}
	for i, l := range s.NVLinks {
		if l.A < 0 || l.B < 0 || l.A >= len(s.Devices) || l.B >= len(s.Devices) || l.A == l.B {
			return fmt.Errorf("NVLinks[%d]: invalid device pair", i)
		}
	}
	return nil
}

// Node declares the network through the same extension API custom connectors use.
func Node(spec Spec) *blueprint.NetworkNode {
	members := append([]messaging.RemotePort(nil), spec.RootPorts...)
	for _, d := range spec.Devices {
		members = append(members, d.Ports...)
	}
	return blueprint.NewNetwork(spec, members, assemble)
}
func assemble(spec Spec, s modeling.Simulation, name string, ports []messaging.Port) {
	c := MakeBuilder().WithSimulation(s).WithFreq(spec.Freq).WithFlitSize(spec.FlitSize).
		WithPCIeSwitchLatency(spec.PCIeSwitchLatency).WithNVLinkSwitchLatency(spec.NVLinkSwitchLatency).
		WithInputWidth(spec.InputWidth).WithOutputWidth(spec.OutputWidth).Build(name)
	offset := len(spec.RootPorts)
	switches := make([]int, spec.NumSwitches+1)
	switches[0] = c.AddRootComplex(ports[:offset])
	for i := 1; i < len(switches); i++ {
		switches[i] = c.AddSwitch()
	}
	for _, l := range spec.PCIeLinks {
		c.ConnectSwitchesWithPCIeLink(switches[l.A], switches[l.B])
	}
	devices := make([]int, len(spec.Devices))
	for i, d := range spec.Devices {
		next := offset + len(d.Ports)
		devices[i] = c.ConnectDevice(switches[d.Switch], ports[offset:next])
		offset = next
	}
	for _, l := range spec.NVLinks {
		c.ConnectDevicesWithNVLink(devices[l.A], devices[l.B])
	}
}
