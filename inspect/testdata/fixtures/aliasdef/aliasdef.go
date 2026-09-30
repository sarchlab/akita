// Package aliasdef declares its definition through a type alias. The
// inspector must see through the alias.
package aliasdef

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	Depth int `json:"depth"`
}

// Def aliases the definition type.
type Def = modeling.ComponentDef[Spec]

// Definition declares the component through the alias.
var Definition = Def{Name: "AliasDef", DefaultSpec: Spec{Depth: 8}}
