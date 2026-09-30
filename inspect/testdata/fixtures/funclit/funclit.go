// Package funclit sets NewMiddlewares to a function literal. The inspector
// must reject it: NewMiddlewares must name a function.
package funclit

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct{ N int }

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: func(*Comp) Middlewares { return Middlewares{} },
}
