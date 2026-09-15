// Package instantiated is a synthetic component whose DefineComponent call
// uses explicit type instantiation instead of inference. The inspector must
// handle both spellings.
package instantiated

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	// Depth is the queue depth.
	Depth int `json:"depth"`
}

// Definition declares the component with explicit type arguments.
var Definition = modeling.DefineComponent[Spec, modeling.None](
	modeling.ComponentDef[Spec, modeling.None]{
		Name:        "Instantiated",
		DefaultSpec: Spec{Depth: 16},
	})
