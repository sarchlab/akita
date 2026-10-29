package mesh

import (
	"fmt"
	"github.com/sarchlab/akita/v5/blueprint"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Tile declares devices at one mesh coordinate. Slice order is binding order.
// Repeated coordinates append devices just like Connector.AddTile.
type Tile struct {
	X     int                    `json:"x"`
	Y     int                    `json:"y"`
	Z     int                    `json:"z"`
	Ports []messaging.RemotePort `json:"ports"`
}

// Spec declares a mesh and its ordered tile additions.
type Spec struct {
	Freq          timing.Freq
	FlitSize      int
	SwitchLatency int
	InputWidth    int
	OutputWidth   int
	Tiles         []Tile
}

// DefaultSpec returns independent mesh settings matching MakeBuilder.
func DefaultSpec() Spec {
	return Spec{Freq: timing.GHz, FlitSize: 16, SwitchLatency: 0, InputWidth: 1, OutputWidth: 1}
}

// Validate checks network settings without creating topology.
func (s Spec) Validate() error {
	if s.Freq <= 0 || s.FlitSize <= 0 || s.InputWidth <= 0 || s.OutputWidth <= 0 || s.SwitchLatency < 0 {
		return fmt.Errorf("mesh parameters: frequency, flit size and widths must be positive; latency nonnegative")
	}
	for i, t := range s.Tiles {
		if t.X < 0 || t.Y < 0 || t.Z < 0 {
			return fmt.Errorf("Tiles[%d]: coordinates must be nonnegative", i)
		}
	}
	return nil
}

// Node declares a mesh; no connector or runtime object exists until Initialize.
func Node(spec Spec) *blueprint.NetworkNode {
	var members []messaging.RemotePort
	for _, t := range spec.Tiles {
		members = append(members, t.Ports...)
	}
	return blueprint.NewNetwork(spec, members, assemble)
}
func assemble(spec Spec, s modeling.Simulation, name string, ports []messaging.Port) {
	c := MakeBuilder().WithSimulation(s).WithFreq(spec.Freq).WithFlitSize(spec.FlitSize).
		WithSwitchLatency(spec.SwitchLatency).WithInputWidth(spec.InputWidth).
		WithOutputWidth(spec.OutputWidth).Build(name)
	offset := 0
	for _, t := range spec.Tiles {
		next := offset + len(t.Ports)
		c.AddTile([3]int{t.X, t.Y, t.Z}, ports[offset:next])
		offset = next
	}
}
