// Package omitzeroonly has a Spec that serializes as {} when zero. The
// inspector must reject it.
package omitzeroonly

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

// Spec's only exported field is omitted when zero, so its zero value
// serializes as {} and the unexported field would be lost.
type Spec struct {
	scratch int
	Count   int `json:",omitzero"`
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
