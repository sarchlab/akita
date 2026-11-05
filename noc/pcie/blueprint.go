package pcie

import (
	"fmt"

	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Switch declares a PCIe switch. Entry zero is the
// root complex (Parent=-1); every other Parent indexes an earlier switch.
type Switch struct {
	Parent int `json:"parent"`
}

// Device declares one endpoint attached to the indexed switch. Multiple devices
// can attach to the same switch; their slice order is endpoint binding order.
type Device struct {
	Switch int                    `json:"switch"`
	Ports  []messaging.RemotePort `json:"ports"`
}

// Spec declares a PCIe tree in deterministic switch and device binding order.
type Spec struct {
	Freq          timing.Freq
	FlitSize      int
	SwitchLatency int
	InputWidth    int
	OutputWidth   int
	Switches      []Switch
	RootPorts     []messaging.RemotePort
	Devices       []Device
}

// DefaultSpec returns settings matching MakeBuilder; add a root before use.
func DefaultSpec() Spec {
	return Spec{Freq: timing.GHz, FlitSize: 34, SwitchLatency: 140, InputWidth: 1, OutputWidth: 1}
}

// Validate checks settings and parent indices without creating topology.
func (s Spec) Validate() error {
	if s.Freq <= 0 || s.FlitSize <= 0 || s.InputWidth <= 0 || s.OutputWidth <= 0 || s.SwitchLatency < 0 {
		return fmt.Errorf("PCIe parameters: frequency, flit size and widths must be positive; latency nonnegative")
	}
	if len(s.Switches) == 0 {
		return fmt.Errorf("Switches: root complex is required")
	}
	for i, sw := range s.Switches {
		if (i == 0 && sw.Parent != -1) || (i > 0 && (sw.Parent < 0 || sw.Parent >= i)) {
			return fmt.Errorf("Switches[%d].Parent: root requires -1; child requires an earlier switch", i)
		}
	}
	for i, d := range s.Devices {
		if d.Switch < 0 || d.Switch >= len(s.Switches) {
			return fmt.Errorf("Devices[%d].Switch: unknown switch", i)
		}
	}
	return nil
}

// Node declares a PCIe tree using the public network extension contract.
func Node(spec Spec) *blueprint.NetworkNode {
	members := append([]messaging.RemotePort(nil), spec.RootPorts...)
	for _, d := range spec.Devices {
		members = append(members, d.Ports...)
	}
	return blueprint.NewNetwork(spec, members, assemble)
}
func assemble(spec Spec, s modeling.Simulation, name string, ports []messaging.Port) {
	c := MakeBuilder().WithSimulation(s).WithFreq(spec.Freq).WithFlitSize(spec.FlitSize).
		WithSwitchLatency(spec.SwitchLatency).WithInputWidth(spec.InputWidth).
		WithOutputWidth(spec.OutputWidth).Build(name)
	ids := make([]int, len(spec.Switches))
	offset := len(spec.RootPorts)
	ids[0] = c.AddRootComplex(ports[:offset])
	for i := 1; i < len(spec.Switches); i++ {
		ids[i] = c.AddSwitch(ids[spec.Switches[i].Parent])
	}
	for _, d := range spec.Devices {
		next := offset + len(d.Ports)
		c.ConnectDevice(ids[d.Switch], ports[offset:next])
		offset = next
	}
}
