// Package badcount is a synthetic component whose port group binds a
// CountField that no Spec field matches. The inspector must reject it.
//
// Do not import this package: the same violation makes DefineComponent panic
// at init, which is exactly the runtime half of the contract. The inspector
// test only reads its source.
package badcount

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Definition binds a CountField that does not exist in Spec.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name: "BadCount",
	PortGroups: []modeling.PortGroupDef{
		{Name: "Out", CountField: "missing"},
	},
})
