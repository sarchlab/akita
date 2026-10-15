package noexecution

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct {
	N int `json:"n"`
}
type Comp = event.Component[Spec, modeling.None, modeling.None, modeling.None, modeling.None]

func init()             { panic("inspector must not execute package initialization") }
func DefaultSpec() Spec { return Spec{N: 17} }
