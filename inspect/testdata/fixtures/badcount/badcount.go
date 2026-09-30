// Package badcount is a synthetic component whose port group binds a
// CountField that no Spec field matches. The inspector must reject it.
package badcount

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Definition binds a CountField that does not exist in Spec.
var Definition = modeling.ComponentDef[Spec]{
	Name: "BadCount",
	PortGroups: []modeling.PortGroupDef{
		{Name: "Out", CountField: "missing"},
	},
}
