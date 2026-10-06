// Package scalars is a synthetic component covering scalar defaults whose
// static and runtime representations are easy to get wrong, and untyped
// ports: Ports fields without a role tag.
package scalars

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Choice string

const Escaped Choice = "quote\"\n\t\\中文"

type Spec struct {
	StringNumber uint64 `json:",string"`
	Fraction     float32
	Choice       Choice
	Quoted       int `json:"'"`
	Lanes        []int
	Targets      []messaging.RemotePort
}

type State struct{}

type Ports struct {
	Untyped      messaging.Port
	UntypedGroup []messaging.Port
}

type Middlewares struct {
	Idle idleMW
}

type idleMW struct{}

func (idleMW) Handle(timing.Event) bool { return false }

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{
		StringNumber: 18446744073709551615, Fraction: 0.1, Choice: Escaped,
		Lanes: []int{1, 2},
	},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
