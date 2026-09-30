package modeling

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// PortDef declares one port and the protocol role(s) it speaks. A port with
// no roles is untyped.
type PortDef struct {
	Name  string
	Roles []*messaging.Role

	// Group declares a dynamically-sized port group instead of a single
	// port. Its size is decided at configuration time: members are addressed
	// "Name[0]", "Name[1]", ... in the order they are assigned (see
	// messaging.PortOwnerBase), and all of them speak Roles.
	Group bool
}

// ComponentDef describes a component's default Spec and port topology. S is
// the Spec type. Tooling finds the component's Resources type through its
// builder's WithResources method, so the definition does not repeat it.
//
// Declare it as a package-level var named Definition in the component's
// package:
//
//	var Definition = modeling.ComponentDef[Spec]{
//	    Name:        "TLB",
//	    DefaultSpec: Spec{Freq: 1 * timing.GHz, NumSets: 1},
//	    Ports: []modeling.PortDef{
//	        {Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
//	    },
//	}
//
// The initializer must be statically evaluable: a keyed composite literal
// with constant leaves (plus role identifiers). Tooling reads and validates
// the same literal without executing the package. Treat the definition as
// read-only so the static and runtime views agree. Builders start from
// DefaultSpec and pass the definition to modeling.Builder.WithDefinition,
// which declares the component's ports.
type ComponentDef[S any] struct {
	// Name is the component's display name, e.g. "TLB".
	Name string

	// DefaultSpec is the component's default configuration. It may set only
	// scalar fields; slice, map and array fields stay unset (and are filled
	// in Build when needed). Reading it therefore yields an independent copy
	// that callers can customize.
	DefaultSpec S

	// Ports declares the component's boundary ports and port groups.
	Ports []PortDef
}

// declarePorts declares every port and port group of the definition on the
// given component. Builder.Build and EventDrivenBuilder.Build call it for the
// definition passed to WithDefinition.
func (d ComponentDef[S]) declarePorts(po messaging.PortOwner) {
	for _, p := range d.Ports {
		if p.Group {
			po.DeclarePortGroup(p.Name, copyRoles(p.Roles)...)
		} else {
			po.DeclarePort(p.Name, copyRoles(p.Roles)...)
		}
	}
}

func copyRoles(roles []*messaging.Role) []*messaging.Role {
	if roles == nil {
		return nil
	}

	out := make([]*messaging.Role, len(roles))
	copy(out, roles)

	return out
}
