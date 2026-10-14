package flatlinks

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type Link struct {
	Remote  string `json:"remote"`
	Latency int    `json:"latency" akita:"min=0"`
}
type Spec struct {
	Freq  timing.Freq `json:"freq"`
	Links []Link      `json:"links"`
}
type Comp = ticking.Component[Spec, modeling.None, modeling.None, modeling.None, modeling.None]

var Definition = ticking.Definition[Spec, modeling.None, modeling.None, modeling.None, modeling.None]{
	DefaultSpec:    Spec{Freq: timing.GHz, Links: []Link{{Remote: "Peer.Top", Latency: 3}, {Remote: "Other.Top"}}},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) modeling.None { return modeling.None{} }
