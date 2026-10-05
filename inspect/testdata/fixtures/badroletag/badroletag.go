// Package badroletag tags a port with a role that names no protocol. The
// inspector must reject it: a role tag is role=<protocol>.<role>.
package badroletag

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct{ N int }

type State struct{}

type Ports struct {
	Top messaging.Port `akita:"role=responder"`
}

type Middlewares struct{}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
