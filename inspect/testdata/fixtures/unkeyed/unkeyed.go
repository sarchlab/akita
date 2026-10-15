// Package unkeyed is a synthetic component whose DefaultSpec literal uses
// positional instead of keyed fields. The inspector must reject it: keyed
// literals are part of the static-analyzability contract.
package unkeyed

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{1} }
