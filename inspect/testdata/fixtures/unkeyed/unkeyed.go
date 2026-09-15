// Package unkeyed is a synthetic component whose DefaultSpec literal uses
// positional instead of keyed fields. The inspector must reject it: keyed
// literals are part of the static-analyzability contract.
package unkeyed

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Definition uses a positional Spec literal on purpose.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name:        "Unkeyed",
	DefaultSpec: Spec{1},
})
