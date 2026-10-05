// Package zerodefaults leaves DefaultSpec out, so every Spec field defaults to
// its zero value, and declares no ports.
package zerodefaults

import (
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
)

type Spec struct {
	Count   int
	Label   string
	Enabled bool
	Ratio   float32
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
