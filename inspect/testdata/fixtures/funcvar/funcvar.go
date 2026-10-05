// Package funcvar sets NewState to a var holding a function. The inspector
// must reject it: NewState must name a function, whose value cannot change.
package funcvar

import (
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
)

type Spec struct{ N int }

type (
	State       struct{ Count int }
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// newState is deliberately a var, not a function.
var newState = func(*Comp) State { return State{Count: 1} }

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
