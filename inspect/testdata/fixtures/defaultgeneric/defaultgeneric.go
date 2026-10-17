package defaultgeneric

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct {
	N int `json:"n"`
}
type Comp = event.Component[Spec, modeling.None, modeling.None, modeling.None, modeling.None]

func DefaultSpec[T any]() Spec { return Spec{N: 1} }
