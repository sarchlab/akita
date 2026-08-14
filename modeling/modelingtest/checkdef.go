// Package modelingtest provides test helpers for component definitions.
package modelingtest

import (
	"encoding/json"
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

// checkDefaults compares the statically extracted defaults with the runtime
// DefaultSpec. Both sides are normalized through JSON so numeric types
// compare by value. A field the runtime marshaling drops (omitempty) must be
// zero on the static side.
func checkDefaults(t *testing.T, static schema.Definition, runtimeSpec any) {
	t.Helper()

	staticDefaults := map[string]any{}
	for _, f := range static.Spec {
		if f.JSONName != "" {
			staticDefaults[f.JSONName] = f.Default
		}
	}

	staticNorm := jsonNormalize(t, staticDefaults)
	runtimeNorm := jsonNormalize(t, runtimeSpec)

	for name, runtimeVal := range runtimeNorm {
		staticVal, ok := staticNorm[name]
		if !ok {
			t.Errorf("default %q: present at runtime, missing statically",
				name)
			continue
		}
		if !reflect.DeepEqual(staticVal, runtimeVal) {
			t.Errorf("default %q: static %v, runtime %v",
				name, staticVal, runtimeVal)
		}
	}

	for name, staticVal := range staticNorm {
		if _, ok := runtimeNorm[name]; !ok && !isJSONZero(staticVal) {
			t.Errorf("default %q: static %v, dropped by runtime marshaling",
				name, staticVal)
		}
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

// jsonNormalize round-trips a value through JSON into a generic map so that
// static (int64/uint64) and runtime (typed struct) values compare by their
// JSON representation.
func jsonNormalize(t *testing.T, v any) map[string]any {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling %T: %v", v, err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshaling %T: %v", v, err)
	}

	return out
}

func isJSONZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case float64:
		return x == 0
	case string:
		return x == ""
	default:
		return false
	}
}
