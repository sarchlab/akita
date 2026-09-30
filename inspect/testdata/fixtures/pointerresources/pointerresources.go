// Package pointerresources has a builder whose WithResources takes a pointer.
// The inspector must still report the Resources fields.
package pointerresources

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
)

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Resources wires the component.
type Resources struct {
	// Storage is the backing store.
	Storage *mem.Storage
}

// Builder builds the component.
type Builder struct {
	resources *Resources
}

// WithResources sets the wiring.
func (b Builder) WithResources(r *Resources) Builder {
	b.resources = r
	return b
}

// Definition declares the component.
var Definition = modeling.ComponentDef[Spec]{Name: "PointerResources"}
