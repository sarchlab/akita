// Package unknownrole tags a port with a role that its protocol does not
// define. The inspector must reject it: a role tag names a role declared with
// messaging.DefineProtocol.
package unknownrole

import (
	_ "github.com/sarchlab/akita/v5/mem/memprotocol" // Declares mem.
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

type Spec struct{ N int }

type State struct{}

type Ports struct {
	Top    messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
	Bottom messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.owner"`
}

type Middlewares struct{}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
