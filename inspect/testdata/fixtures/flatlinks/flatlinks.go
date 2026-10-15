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
type Routing struct {
	Destinations []string `json:"destinations"`
	Size         uint64   `json:"size" akita:"min=1"`
}
type Spec struct {
	Bottom   Routing     `json:"bottom"`
	Mappings []Routing   `json:"mappings"`
	Freq     timing.Freq `json:"freq"`
	Links    []Link      `json:"links"`
}
type Comp = ticking.Component[Spec, modeling.None, modeling.None, modeling.None, modeling.None]

var Definition = ticking.Definition[Spec, modeling.None, modeling.None, modeling.None, modeling.None]{
	DefaultSpec:    Spec{Bottom: Routing{Destinations: []string{"A", "B"}, Size: 64}, Mappings: []Routing{{Destinations: []string{"C"}}}, Freq: timing.GHz, Links: []Link{{Remote: "Peer.Top", Latency: 3}, {Remote: "Other.Top"}}},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) modeling.None { return modeling.None{} }
