// Package badstateinitializer declares an initializer with the wrong signature.
package badstateinitializer

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct{ N int }
type State struct{ Count int }
type Ports struct{}
type Middlewares struct{}
type Comp = event.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = event.Definition[Spec, State, modeling.None, Ports, Middlewares]{}

func (*State) Initialize() {}
