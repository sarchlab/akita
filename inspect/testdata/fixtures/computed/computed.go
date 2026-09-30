// Package computed is a synthetic component whose Definition is returned by
// a function instead of written as a literal. The inspector must reject it:
// only a literal can be read without executing the package.
package computed

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

func makeDefinition() modeling.ComponentDef[Spec] {
	return modeling.ComponentDef[Spec]{Name: "Computed"}
}

// Definition is computed on purpose.
var Definition = makeDefinition()
