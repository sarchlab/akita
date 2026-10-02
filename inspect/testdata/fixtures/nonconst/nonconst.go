// Package nonconst is a synthetic component whose definition is valid Go and
// works at runtime, but is not statically analyzable: a default comes from a
// package-level var. The inspector must reject it.
package nonconst

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

// Spec configures the component.
type Spec struct {
	NumLanes int `json:"num_lanes"`
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// dynamicLanes is deliberately a var, not a const.
var dynamicLanes = 4

// Definition declares the component with a non-constant default.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{NumLanes: dynamicLanes},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
