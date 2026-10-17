// Package aliasdef declares Comp through another type alias.
package aliasdef

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Spec configures the component.
type Spec struct {
	Freq  timing.Freq `json:"freq"`
	Depth int         `json:"depth"`
}

type (
	// State is the mutable runtime state.
	State struct{}

	// Ports holds the component's ports.
	Ports struct{}

	// Middlewares holds the component's behavior.
	Middlewares struct{}
)

// Def aliases the component type.
type Def = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Comp is the component.
type Comp = Def

// DefaultSpec returns the default configuration.
func DefaultSpec() Spec { return Spec{Freq: timing.GHz, Depth: 8} }
