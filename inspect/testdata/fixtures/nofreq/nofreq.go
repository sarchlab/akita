// Package nofreq declares a ticking definition without a Freq field.
// The inspector and CheckTicking must reject it.
package nofreq

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

// Spec configures the component.
type Spec struct {
	Depth int `json:"depth"`
}

type (
	// State is the mutable runtime state.
	State struct{}

	// Ports holds the component's ports.
	Ports struct{}

	// Middlewares holds the component's behavior.
	Middlewares struct{}
)

// Def aliases the definition type.
type Def = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Comp is the component.
type Comp = Def

// DefaultSpec returns the default configuration.
func DefaultSpec() Spec { return Spec{Depth: 8} }
