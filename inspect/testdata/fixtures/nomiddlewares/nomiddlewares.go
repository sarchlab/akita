// Package nomiddlewares uses zero State and Middlewares without initializers.
package nomiddlewares

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct{ N int }

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

var Definition = event.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{N: 1},
}
