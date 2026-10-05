// Package badresources passes a pointer as the Resources type argument. The
// inspector must reject it: Resources is a struct whose fields it reports.
package badresources

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
)

type Spec struct{ N int }

// Resources wires the component.
type Resources struct {
	Storage *mem.Storage
}

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, *Resources, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, *Resources, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
