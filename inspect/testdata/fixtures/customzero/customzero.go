// Package customzero hides distinct Spec values behind a custom IsZero.
// The inspector must reject the omitzero option on that field.
package customzero

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type customZero int

func (v customZero) IsZero() bool { return v < 10 }

type Spec struct {
	N     int        `json:"n"`
	Value customZero `json:",omitzero"`
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{} }
