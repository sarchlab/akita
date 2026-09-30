// Package containerdefault is a synthetic component whose DefaultSpec sets a
// slice field. The inspector must reject it: a container default would be
// shared by every copy of the Spec.
package containerdefault

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	Latencies []int `json:"latencies"`
}

// Definition sets a container default on purpose.
var Definition = modeling.ComponentDef[Spec]{
	Name:        "ContainerDefault",
	DefaultSpec: Spec{Latencies: []int{2, 4}},
}
