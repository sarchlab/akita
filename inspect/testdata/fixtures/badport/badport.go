// Package badport has a Ports field that is not a port. The inspector must
// reject it: every Ports field is a messaging.Port or a []messaging.Port.
package badport

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type Spec struct{ N int }

type State struct{}

type Ports struct {
	Top   messaging.Port
	Depth int
}

type Middlewares struct{}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
