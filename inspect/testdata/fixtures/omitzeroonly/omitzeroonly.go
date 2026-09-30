package omitzeroonly

import "github.com/sarchlab/akita/v5/modeling"

// Spec's only exported field is omitted when zero, so its zero value
// serializes as {} and the unexported field would be lost.
type Spec struct {
	scratch int
	Count   int `json:",omitzero"`
}

var Definition = modeling.ComponentDef[Spec]{Name: "C"}
