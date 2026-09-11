// Package nonconst is a synthetic component whose definition is valid Go and
// works at runtime, but is not statically analyzable: a default comes from a
// package-level var. The inspector must reject it.
package nonconst

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	NumLanes int `json:"num_lanes"`
}

// dynamicLanes is deliberately a var, not a const.
var dynamicLanes = 4

// Definition declares the component with a non-constant default.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name:        "NonConst",
	DefaultSpec: Spec{NumLanes: dynamicLanes},
})
