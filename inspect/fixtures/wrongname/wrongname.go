// Package wrongname is a synthetic component whose definition var violates
// the naming contract. The inspector must reject it.
package wrongname

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Def is misnamed on purpose: the contract requires "Definition".
var Def = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name: "WrongName",
})
