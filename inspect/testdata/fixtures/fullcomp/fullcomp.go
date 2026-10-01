// Package fullcomp is a synthetic component used by the inspector tests. It
// exercises every feature of the definition schema: docs, units, tags,
// choices, excluded fields, resources, port roles and port groups, and
// middlewares.
package fullcomp

import (
	"github.com/sarchlab/akita/v5/mem"
	_ "github.com/sarchlab/akita/v5/mem/memcontrolprotocol" // Declares mem.control.
	_ "github.com/sarchlab/akita/v5/mem/memprotocol"        // Declares mem.
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling/ticking"
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
type State struct {
	// Served counts the requests served.
	Served int `json:"served"`
}

// Resources wires the component to external objects.
type Resources struct {
	// Mapper locates the remote port that serves a given address.
	Mapper mem.AddressToPortMapper `json:"-"`
}

// Ports holds the component's ports.
type Ports struct {
	// Top receives memory requests.
	Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`

	// Ctrl multiplexes two protocols on one port.
	Ctrl messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder,role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`

	// Out is a port group: its size is decided when it is wired.
	Out []messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}

// Middlewares holds the component's behavior.
type Middlewares struct {
	// Control handles control commands.
	Control *controlMW

	// Serve forwards requests from Top to Out.
	Serve *serveMW
}

// Comp is the component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]

// Definition declares the component.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:     1 * timing.GHz,
		NumLanes: 4,
		Mode:     ModeFast,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newState(*Comp) State { return State{} }

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Control: &controlMW{comp: c}, Serve: &serveMW{comp: c}}
}

type controlMW struct{ comp *Comp }

func (m *controlMW) Handle(timing.Event) bool { return false }

type serveMW struct{ comp *Comp }

func (m *serveMW) Handle(timing.Event) bool { return false }
