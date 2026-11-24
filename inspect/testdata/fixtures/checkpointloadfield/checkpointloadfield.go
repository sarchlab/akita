// Package checkpointloadfield has a State field named like the optional
// checkpoint hook, which is ordinary serializable data rather than the hook.
package checkpointloadfield

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/event"
)

type Spec struct{ N int }
type State struct {
	AfterCheckpointLoad int `json:"after_checkpoint_load"`
}
type Ports struct{}
type Middlewares struct{}
type Comp = event.Component[Spec, State, modeling.None, Ports, Middlewares]

func DefaultSpec() Spec { return Spec{} }
