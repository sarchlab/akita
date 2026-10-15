// Package dashscalar hides a scalar Spec field from the checkpoint hash.
// The inspector must reject the JSON-excluded field.
package dashscalar

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type Spec struct {
	N      int `json:"n"`
	Hidden int `json:"-"`
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{} }
