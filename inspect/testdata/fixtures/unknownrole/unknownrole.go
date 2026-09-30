// Package unknownrole tags a port with a role that its protocol does not
// define. The inspector must reject it: a role tag names a role declared with
// messaging.DefineProtocol.
package unknownrole

import (
	_ "github.com/sarchlab/akita/v5/mem/memprotocol" // Declares mem.
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
)

type Spec struct{ N int }

type State struct{}

type Ports struct {
	Top    messaging.Port `akita:"role=mem/responder"`
	Bottom messaging.Port `akita:"role=mem/owner"`
}

type Middlewares struct{}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
