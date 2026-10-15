// Package pointerdef incorrectly declares Comp as a pointer alias.
package pointerdef

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type Spec struct{ N int }

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = *ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{} }
