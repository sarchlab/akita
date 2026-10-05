// Package dashcontainer hides a map Spec field from JSON with a "-" tag. The
// inspector must still reject it: Spec fields must be scalars or slices of
// scalars.
package dashcontainer

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct {
	N    int            `json:"n"`
	List map[string]int `json:"-"`
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
