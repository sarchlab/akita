// Package marshalercontainer customizes its Spec's JSON but still holds a
// map. The inspector must reject it: Spec fields must be scalars or slices of
// scalars even when the Spec customizes its JSON.
package marshalercontainer

import (
	"encoding/json"

	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/ticking"
)

// Spec customizes its JSON but still holds a slice.
type Spec struct {
	Sizes map[string]int
}

func (s Spec) MarshalJSON() ([]byte, error) { return json.Marshal(s.Sizes) }

func (s *Spec) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &s.Sizes) }

type (
	State       struct{}
	Ports       struct{}
	Middlewares struct{}
)

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
