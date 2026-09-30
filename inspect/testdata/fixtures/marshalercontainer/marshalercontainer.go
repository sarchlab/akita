package marshalercontainer

import (
	"encoding/json"

	"github.com/sarchlab/akita/v5/modeling"
)

// Spec customizes its JSON but still holds a slice.
type Spec struct {
	Sizes []int
}

func (s Spec) MarshalJSON() ([]byte, error) { return json.Marshal(s.Sizes) }

func (s *Spec) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &s.Sizes) }

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
