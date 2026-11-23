// Package valuecheckpointload incorrectly rebinds a copy of State.
package valuecheckpointload

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type State struct{ Value int }
type Comp = event.Component[modeling.None, State, modeling.None, modeling.None, modeling.None]

func DefaultSpec() modeling.None { return modeling.None{} }

func (State) AfterCheckpointLoad(*Comp) {}
