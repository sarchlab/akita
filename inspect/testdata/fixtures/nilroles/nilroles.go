// Package nilroles declares an untyped port with Roles: nil.
package nilroles

import "github.com/sarchlab/akita/v5/modeling"

// Spec configures the component.
type Spec struct {
	N int `json:"n"`
}

// Definition declares one untyped port.
var Definition = modeling.ComponentDef[Spec]{
	Name:  "NilRoles",
	Ports: []modeling.PortDef{{Name: "Untyped", Roles: nil}},
}
