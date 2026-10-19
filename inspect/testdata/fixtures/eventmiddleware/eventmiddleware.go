package eventmiddleware

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Spec struct{ Freq timing.Freq }
type middleware struct{}

func (middleware) Handle(timing.Event) {}

type middlewares struct{ Work middleware }
type Comp = event.Component[Spec, modeling.None, modeling.None, modeling.None, middlewares]

func DefaultSpec() Spec { return Spec{Freq: timing.GHz} }
