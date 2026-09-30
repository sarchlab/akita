package modeling

import (
	"reflect"

	"github.com/sarchlab/akita/v5/messaging"
)

// PortDef declares one port and the protocol role(s) it speaks. A port with
// no roles is untyped.
type PortDef struct {
	Name  string
	Roles []*messaging.Role
}

// PortGroupDef declares a dynamically-sized group of ports that all speak the
// same role(s). The group's size is not part of the definition: it is decided
// at configuration time, and members are addressed "Name[0]", "Name[1]", ...
// in the order they are assigned (see messaging.PortOwnerBase).
type PortGroupDef struct {
	Name  string
	Roles []*messaging.Role

	// MinCount and MaxCount bound how many members a configuration may give
	// the group. MaxCount of 0 means unbounded.
	MinCount int
	MaxCount int

	// CountField optionally names the Spec field (by its JSON name) that
	// holds the group's configured size, for components that size internal
	// structures by port count at Build time. The field must be an integer
	// tagged `akita:"derived"`: the wired topology is the source of truth
	// for the count, so the field must not also be user-configurable. Empty
	// means the count is implicit in how many members get assigned.
	CountField string
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
// read-only so the static and runtime views agree. Builders use NewSpec to
// obtain a configuration to edit and pass the definition to
// modeling.Builder.WithDefinition, which declares the component's ports.
type ComponentDef[S any] struct {
	// Name is the component's display name, e.g. "TLB".
	Name string

	// DefaultSpec is the component's default configuration. Use NewSpec to
	// obtain a copy for customization.
	DefaultSpec S

	// Ports and PortGroups declare the component's boundary ports.
	Ports      []PortDef
	PortGroups []PortGroupDef
}

// NewSpec returns a copy of the default configuration, recursively copying
// exported slice, map, and array contents. Callers can customize the result
// and pass it to the builder's WithSpec without changing the defaults.
func (d ComponentDef[S]) NewSpec() S {
	v := deepCopyStruct(reflect.ValueOf(d.DefaultSpec))
	return v.Interface().(S)
}

// declarePorts declares every port and port group of the definition on the
// given component. Builder.Build and EventDrivenBuilder.Build call it for the
// definition passed to WithDefinition.
func (d ComponentDef[S]) declarePorts(po messaging.PortOwner) {
	for _, p := range d.Ports {
		po.DeclarePort(p.Name, copyRoles(p.Roles)...)
	}

	for _, g := range d.PortGroups {
		po.DeclarePortGroup(g.Name, copyRoles(g.Roles)...)
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

// deepCopyStruct copies the Spec's exported fields, recursively cloning its
// slice, map and array contents. ValidateSpec permits nested containers.
func deepCopyStruct(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := range v.NumField() {
			if out.Field(i).CanSet() {
				out.Field(i).Set(deepCopyStruct(v.Field(i)))
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(deepCopyStruct(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(deepCopyStruct(v.Index(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		it := v.MapRange()
		for it.Next() {
			out.SetMapIndex(it.Key(), deepCopyStruct(it.Value()))
		}
		return out
	default:
		return v
	}
}
