// Package nonfinite bounds a Spec field with a non-finite minimum. The
// inspector must reject it.
package nonfinite

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct {
	N int `akita:"min=NaN"`
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
