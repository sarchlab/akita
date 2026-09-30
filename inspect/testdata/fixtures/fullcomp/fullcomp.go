// Package fullcomp is a synthetic component used by the inspector tests. It
// exercises every feature of the definition schema: docs, units, tags,
// choices, excluded fields, resources, and port groups.
package fullcomp

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Mode selects how precisely the component is modeled.
type Mode string

const (
	// ModeFast trades accuracy for speed.
	ModeFast Mode = "fast"
	// ModeAccurate models every cycle.
	ModeAccurate Mode = "accurate"
)

// Spec configures the component.
type Spec struct {
	// Freq is the clock frequency.
	Freq timing.Freq `json:"freq"`

	// NumLanes is the number of parallel lanes.
	NumLanes int `json:"num_lanes" akita:"min=1,max=64"`

	// NumOut is computed in Build from the number of wired Out ports.
	NumOut int `json:"num_out" akita:"derived"`

	// Mode selects how precisely the component is modeled.
	Mode Mode `json:"mode"`

	Hidden int `json:"-"`
}

// State is the mutable runtime state.
type State struct{}

// Resources wires the component to external objects.
type Resources struct {
	// Mapper locates the remote port that serves a given address.
	Mapper mem.AddressToPortMapper `json:"-"`
}

// Builder builds the component. Its WithResources parameter tells the
// inspector which type holds the component's resources.
type Builder struct {
	resources Resources
}

// WithResources sets the external wiring.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Definition declares the component.
var Definition = modeling.ComponentDef[Spec]{
	Name: "FullComp",
	DefaultSpec: Spec{
		Freq:     1 * timing.GHz,
		NumLanes: 4,
		Mode:     ModeFast,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		// Ctrl multiplexes two protocols on one port.
		{Name: "Ctrl", Roles: []*messaging.Role{
			memprotocol.Responder, memcontrolprotocol.Responder}},
		// Out is a port group: its size is decided when it is wired.
		{Name: "Out", Roles: []*messaging.Role{memprotocol.Requester}, Group: true},
	},
}
