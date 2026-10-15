// Package valueinitializer incorrectly initializes a copy of State.
package valueinitializer

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type State struct{ Value int }
type Comp = event.Component[modeling.None, State, modeling.None, modeling.None, modeling.None]

var Definition = event.Definition[modeling.None, State, modeling.None, modeling.None, modeling.None]{}

func (State) Initialize(*Comp) {}
