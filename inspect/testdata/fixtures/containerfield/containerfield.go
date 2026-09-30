// Package containerfield is a synthetic component whose Spec has a slice
// field. The inspector must reject it: Spec fields must be scalars.
package containerfield

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	Latencies []int `json:"latencies"`
}

// Definition declares the component.
var Definition = modeling.ComponentDef[Spec]{Name: "ContainerField"}
