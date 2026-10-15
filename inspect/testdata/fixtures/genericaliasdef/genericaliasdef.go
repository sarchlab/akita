// Package genericaliasdef declares its definition through a generic type
// alias. The inspector must see through the alias.
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

// Def is a generic alias of the definition type.
type Def[S any] = ticking.Definition[S, State, modeling.None, Ports, Middlewares]

// Comp is the component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the component through the generic alias.
var Definition = Def[Spec]{DefaultSpec: Spec{Freq: timing.GHz, Depth: 4}}
