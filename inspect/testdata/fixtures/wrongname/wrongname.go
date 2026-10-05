// Package wrongname is a synthetic component whose definition var violates
// the naming contract. The inspector must reject it.
package wrongname

import (
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
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

// Def is misnamed on purpose: the contract requires "Definition".
var Def = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
