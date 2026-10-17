// Package computed returns computed defaults, which static inspection rejects.
package computed

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

func makeSpec() Spec { return Spec{} }

func DefaultSpec() Spec { return makeSpec() }
