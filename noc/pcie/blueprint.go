package pcie

import (
	"fmt"
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Switch declares a PCIe switch and its attached devices. Entry zero is the
// root complex (Parent=-1); every other Parent indexes an earlier switch.
type Switch struct {
	Parent int                    `json:"parent"`
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
	return nil
}

// Node declares a PCIe tree using the public network extension contract.
func Node(spec Spec) *blueprint.NetworkNode {
	var members []messaging.RemotePort
	for _, sw := range spec.Switches {
		members = append(members, sw.Ports...)
	}
	return blueprint.NewNetwork(spec, members, assemble)
}
func assemble(spec Spec, s modeling.Simulation, name string, ports []messaging.Port) {
	c := MakeBuilder().WithSimulation(s).WithFreq(spec.Freq).WithFlitSize(spec.FlitSize).
		WithSwitchLatency(spec.SwitchLatency).WithInputWidth(spec.InputWidth).
		WithOutputWidth(spec.OutputWidth).Build(name)
	ids := make([]int, len(spec.Switches))
	offset := 0
	for i, sw := range spec.Switches {
		next := offset + len(sw.Ports)
		if i == 0 {
			ids[i] = c.AddRootComplex(ports[offset:next])
		} else {
			ids[i] = c.AddSwitch(ids[sw.Parent])
			c.ConnectDevice(ids[i], ports[offset:next])
		}
		offset = next
	}
}
