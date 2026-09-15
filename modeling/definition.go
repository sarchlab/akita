package modeling

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sarchlab/akita/v5/messaging"
)

// PortDeclarer is the subset of a component's port API that DeclarePorts
// needs. *messaging.PortOwnerBase satisfies it, and so does every component
// that embeds one.
type PortDeclarer interface {
	DeclarePort(name string, roles ...*messaging.Role)
	DeclarePortGroup(name string, roles ...*messaging.Role)
}

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
// the Spec type and R the builder Resources type: the external references
// supplied at construction, even when the component does not retain them.
//
// Declare it as a package-level var in the component's package:
//
//	var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
//	    Name:        "TLB",
//	    DefaultSpec: Spec{Freq: 1 * timing.GHz, NumSets: 1},
//	    Ports: []modeling.PortDef{
//	        {Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
//	    },
//	})
//
// The initializer must be statically evaluable: a single DefineComponent call
// whose argument is a composite literal with constant leaves (plus role
// identifiers). Tooling reads the same literal without executing the package.
// Treat the definition as read-only after initialization so the static and
// runtime views agree. Builders use NewSpec to obtain a configuration to edit
// and DeclarePorts to declare the component's boundary.
type ComponentDef[S, R any] struct {
	// Name is the component's display name, e.g. "TLB".
	Name string

	// DefaultSpec is the component's default configuration. Use NewSpec to
	// obtain a copy for customization.
	DefaultSpec S

	// Ports and PortGroups declare the component's boundary ports.
	Ports      []PortDef
	PortGroups []PortGroupDef
}

// DefineComponent validates def and returns it unchanged. It
// panics on an invalid definition — like DefineProtocol, it is meant to run
// as a package-level var initializer, where a bad definition is a programming
// error that must fail loudly at init.
func DefineComponent[S, R any](def ComponentDef[S, R]) ComponentDef[S, R] {
	if def.Name == "" {
		panic("modeling: component definition must have a name")
	}

	if err := ValidateSpec(def.DefaultSpec); err != nil {
		panic(fmt.Sprintf(
			"modeling: component %q has an invalid default Spec: %v",
			def.Name, err))
	}

	specFields := validateSpecFieldTags[S](def.Name)
	validatePortDefs(def.Name, def.Ports, def.PortGroups, specFields)

	return def
}

// NewSpec returns a copy of the default configuration, recursively copying
// exported slice, map, and array contents. Callers can customize the result
// and pass it to the builder's WithSpec without changing the defaults.
func (d ComponentDef[S, R]) NewSpec() S {
	v := deepCopyStruct(reflect.ValueOf(d.DefaultSpec))
	return v.Interface().(S)
}

// DeclarePorts declares every port and port group of the definition on the
// given component. Builders call it in Build in place of per-port
// DeclarePort calls.
func (d ComponentDef[S, R]) DeclarePorts(po PortDeclarer) {
	for _, p := range d.Ports {
		po.DeclarePort(p.Name, copyRoles(p.Roles)...)
	}

	for _, g := range d.PortGroups {
		po.DeclarePortGroup(g.Name, copyRoles(g.Roles)...)
	}
}

// specField describes one exported Spec field, as needed to validate the
// definition against the Spec type.
type specField struct {
	jsonName string
	kind     reflect.Kind
	tag      FieldTag
}

// validateSpecFieldTags checks every exported field of S: the akita tag must
// parse, min/max only apply to numeric fields, and JSON names must be unique.
// It returns the fields indexed by JSON name for later cross-checks.
func validateSpecFieldTags[S any](compName string) map[string]specField {
	fields := map[string]specField{}

	t := reflect.TypeFor[S]()
	if t.Kind() != reflect.Struct {
		return fields
	}

	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}

		tag, err := ParseFieldTag(f.Tag.Get("akita"))
		if err != nil {
			panic(fmt.Sprintf(
				"modeling: component %q Spec field %q: %v",
				compName, f.Name, err))
		}

		if (tag.Min != nil || tag.Max != nil) && !isNumericKind(f.Type.Kind()) {
			panic(fmt.Sprintf(
				"modeling: component %q Spec field %q: "+
					"min/max apply only to numeric fields, not %s",
				compName, f.Name, f.Type))
		}

		jsonName := fieldJSONName(f)
		if jsonName == "" {
			continue
		}

		if _, dup := fields[jsonName]; dup {
			panic(fmt.Sprintf(
				"modeling: component %q Spec has duplicate JSON name %q",
				compName, jsonName))
		}

		fields[jsonName] = specField{
			jsonName: jsonName,
			kind:     f.Type.Kind(),
			tag:      tag,
		}
	}

	return fields
}

func validatePortDefs(
	compName string,
	ports []PortDef,
	groups []PortGroupDef,
	specFields map[string]specField,
) {
	seen := map[string]bool{}

	declareName := func(name string) {
		if name == "" {
			panic(fmt.Sprintf(
				"modeling: component %q declares a port with an empty name",
				compName))
		}
		if seen[name] {
			panic(fmt.Sprintf(
				"modeling: component %q declares port %q more than once",
				compName, name))
		}
		seen[name] = true
	}

	checkRoles := func(name string, roles []*messaging.Role) {
		for _, r := range roles {
			if r == nil {
				panic(fmt.Sprintf(
					"modeling: component %q port %q has a nil role",
					compName, name))
			}
		}
	}

	for _, p := range ports {
		declareName(p.Name)
		checkRoles(p.Name, p.Roles)
	}

	for _, g := range groups {
		declareName(g.Name)
		checkRoles(g.Name, g.Roles)
		validatePortGroupDef(compName, g, specFields)
	}
}

func validatePortGroupDef(
	compName string, g PortGroupDef, specFields map[string]specField,
) {
	if g.MinCount < 0 || g.MaxCount < 0 {
		panic(fmt.Sprintf(
			"modeling: component %q port group %q has a negative count bound",
			compName, g.Name))
	}

	if g.MaxCount != 0 && g.MaxCount < g.MinCount {
		panic(fmt.Sprintf(
			"modeling: component %q port group %q has MaxCount %d < MinCount %d",
			compName, g.Name, g.MaxCount, g.MinCount))
	}

	if g.CountField == "" {
		return
	}

	f, ok := specFields[g.CountField]
	if !ok {
		panic(fmt.Sprintf(
			"modeling: component %q port group %q: "+
				"CountField %q does not match any Spec field's JSON name",
			compName, g.Name, g.CountField))
	}

	if !isIntegerKind(f.kind) {
		panic(fmt.Sprintf(
			"modeling: component %q port group %q: "+
				"CountField %q must be an integer Spec field",
			compName, g.Name, g.CountField))
	}

	if !f.tag.Derived {
		panic(fmt.Sprintf(
			"modeling: component %q port group %q: "+
				"CountField %q must be tagged `akita:\"derived\"` — the wired "+
				"topology is the source of truth for the count",
			compName, g.Name, g.CountField))
	}
}

// fieldJSONName returns the name encoding/json uses for the field, or "" if
// the field is excluded from JSON.
func fieldJSONName(f reflect.StructField) string {
	tag, ok := f.Tag.Lookup("json")
	if !ok {
		return f.Name
	}

	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return f.Name
	}

	return name
}

func isNumericKind(k reflect.Kind) bool {
	return isIntegerKind(k) || k == reflect.Float32 || k == reflect.Float64
}

func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
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
