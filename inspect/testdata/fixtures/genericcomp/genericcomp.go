package genericcomp

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct {
	N int `json:"n"`
}
type Comp[S any] = event.Component[S, modeling.None, modeling.None, modeling.None, modeling.None]

func DefaultSpec() Spec { return Spec{N: 1} }
