// Package modelingtest provides test helpers for component definitions.
package modelingtest

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

// CheckDefinition asserts that the inspector's static view of the package at
// pkgPath matches the live Definition value: same name, same defaults, same
// ports and port groups. Every migrated component adds one test calling it,
// which turns "the static and runtime views agree" into a CI guarantee
// instead of a convention.
func CheckDefinition[S, R any](
	t *testing.T, def modeling.Definition[S, R], pkgPath string,
) {
	t.Helper()

	static := staticDefinition(t, pkgPath)

	if static.Name != def.Name() {
		t.Errorf("name: static %q, runtime %q", static.Name, def.Name())
	}

	checkDefaults(t, static, def.DefaultSpec())
	checkPorts(t, static, def.Ports())
	checkPortGroups(t, static, def.PortGroups())
}

func staticDefinition(t *testing.T, pkgPath string) schema.Definition {
	t.Helper()

	defs, errs := inspect.Inspect(inspect.Options{}, pkgPath)
	for _, err := range errs {
		t.Errorf("inspect: %v", err)
	}

	for _, d := range defs {
		if d.Package == pkgPath {
			return d
		}
	}

	t.Fatalf("inspector found no definition in %s", pkgPath)
	return schema.Definition{}
}

// checkDefaults compares Go field values, before encoding/json applies byte
// encoding, string tags or omitempty. This preserves integer precision and
// distinguishes nil containers from empty containers.
func checkDefaults(t *testing.T, static schema.Definition, runtimeSpec any) {
	t.Helper()
	for name, mismatch := range defaultMismatches(static, runtimeSpec) {
		t.Errorf("default %q: %s", name, mismatch)
	}
}

func defaultMismatches(static schema.Definition, runtimeSpec any) map[string]string {
	mismatches := map[string]string{}
	runtime := reflect.ValueOf(runtimeSpec)
	seen := map[string]bool{}
	for _, f := range static.Spec {
		seen[f.Name] = true
		value := runtime.FieldByName(f.Name)
		if !value.IsValid() {
			mismatches[f.Name] = "missing at runtime"
			continue
		}
		actual := defaultValue(value)
		if !reflect.DeepEqual(f.Default, actual) {
			mismatches[f.Name] = fmt.Sprintf("static %v, runtime %v", f.Default, actual)
		}
	}
	for f := range runtime.Type().Fields() {
		if f.IsExported() && !seen[f.Name] {
			mismatches[f.Name] = "missing statically"
		}
	}
	return mismatches
}

// defaultValue uses the inspector's representation of primitive values and
// containers. Maps use decimal integer keys, matching the schema's JSON keys.
func defaultValue(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.String:
		return v.String()
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = defaultValue(v.Index(i))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			out[fmt.Sprint(defaultValue(iter.Key()))] = defaultValue(iter.Value())
		}
		return out
	default:
		return nil
	}
}

func checkPorts(
	t *testing.T, static schema.Definition, runtime []modeling.PortDef,
) {
	t.Helper()

	if len(static.Ports) != len(runtime) {
		t.Errorf("ports: static %d, runtime %d",
			len(static.Ports), len(runtime))
		return
	}

	for i, p := range runtime {
		s := static.Ports[i]
		if s.Name != p.Name ||
			!reflect.DeepEqual(s.Roles, runtimeRoles(p.Roles)) {
			t.Errorf("port %d: static %+v, runtime %s %+v",
				i, s, p.Name, runtimeRoles(p.Roles))
		}
	}
}

func checkPortGroups(
	t *testing.T, static schema.Definition, runtime []modeling.PortGroupDef,
) {
	t.Helper()

	if len(static.PortGroups) != len(runtime) {
		t.Errorf("port groups: static %d, runtime %d",
			len(static.PortGroups), len(runtime))
		return
	}

	for i, g := range runtime {
		s := static.PortGroups[i]
		if s.Name != g.Name || s.MinCount != g.MinCount ||
			s.MaxCount != g.MaxCount || s.CountField != g.CountField ||
			!reflect.DeepEqual(s.Roles, runtimeRoles(g.Roles)) {
			t.Errorf("port group %d: static %+v, runtime %+v", i, s, g)
		}
	}
}

func runtimeRoles(roles []*messaging.Role) []schema.Role {
	if len(roles) == 0 {
		return nil
	}

	out := make([]schema.Role, len(roles))
	for i, r := range roles {
		out[i] = schema.Role{
			Protocol: r.Protocol().Name(),
			Role:     r.Name(),
		}
	}

	return out
}
