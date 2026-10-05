// Package computed is a synthetic component whose Definition is returned by
// a function instead of written as a literal. The inspector must reject it:
// only a literal can be read without executing the package.
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

func makeDefinition() ticking.Definition[Spec, State, modeling.None, Ports, Middlewares] {
	return ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
		NewMiddlewares: newMiddlewares,
	}
}

// Definition is computed on purpose.
var Definition = makeDefinition()

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
