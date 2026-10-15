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

type Comp = event.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{N: 1} }
