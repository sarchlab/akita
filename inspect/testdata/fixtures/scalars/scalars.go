// Package scalars is a synthetic component covering scalar defaults whose
// static and runtime representations are easy to get wrong, and ports with
// empty or omitted roles.
package scalars

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

type Choice string

const Escaped Choice = "quote\"\n\t\\中文"

type Spec struct {
	StringNumber uint64 `json:",string"`
	Fraction     float32
	Choice       Choice
	Quoted       int `json:"'"`
}

var Definition = modeling.ComponentDef[Spec]{
	Name: "Scalars",
	DefaultSpec: Spec{
		StringNumber: 18446744073709551615, Fraction: 0.1, Choice: Escaped,
	},
	Ports: []modeling.PortDef{
		{Name: "Empty", Roles: []*messaging.Role{}}, {Name: "Omitted"},
		{Name: "EmptyGroup", Roles: []*messaging.Role{}, Group: true}, {Name: "OmittedGroup", Group: true},
	},
}
