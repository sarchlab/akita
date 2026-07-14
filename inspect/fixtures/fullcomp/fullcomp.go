// Package fullcomp is a synthetic component used by the inspector tests. It
// exercises every feature of the definition schema: docs, units, tags,
// choices, slice defaults, excluded fields, resources, and port groups.
package fullcomp

import (
	"github.com/sarchlab/akita/v5/mem"
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

	// NumOut is derived from the number of wired Out ports.
	NumOut int `json:"num_out" akita:"derived"`

	// Mode selects how precisely the component is modeled.
	Mode Mode `json:"mode"`

	// Weights are per-lane scheduling weights.
	Weights []float64 `json:"weights"`

	Hidden int `json:"-"`
}

// State is the mutable runtime state.
type State struct{}

// Resources wires the component to external objects.
type Resources struct {
	// Mapper locates the remote port that serves a given address.
	Mapper mem.AddressToPortMapper `json:"-"`
}

// Definition declares the component.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "FullComp",
	DefaultSpec: Spec{
		Freq:     1 * timing.GHz,
		NumLanes: 4,
		Mode:     ModeFast,
		Weights:  []float64{1, 2},
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
	},
	PortGroups: []modeling.PortGroupDef{
		{
			Name:       "Out",
			Roles:      []*messaging.Role{memprotocol.Requester},
			MinCount:   1,
			MaxCount:   8,
			CountField: "num_out",
		},
	},
})
