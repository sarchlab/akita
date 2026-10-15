// Package genericaliasdef declares Comp through a generic alias.
package genericaliasdef

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

// Def is a generic alias of the component type.
type Def[S any] = ticking.Component[S, State, modeling.None, Ports, Middlewares]

// Comp is the component.
type Comp = Def[Spec]

// DefaultSpec returns the default configuration.
func DefaultSpec() Spec { return Spec{Freq: timing.GHz, Depth: 4} }
