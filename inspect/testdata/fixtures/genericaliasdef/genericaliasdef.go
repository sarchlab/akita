// Package genericaliasdef declares its definition through a generic type
// alias. The inspector must see through the alias.
package genericaliasdef

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	Depth int `json:"depth"`
}

// Def is a generic alias of the definition type.
type Def[S any] = modeling.ComponentDef[S]

// Definition declares the component through the generic alias.
var Definition = Def[Spec]{Name: "GenericAliasDef", DefaultSpec: Spec{Depth: 4}}
