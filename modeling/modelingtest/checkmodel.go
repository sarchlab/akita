// Package modelingtest provides test helpers for component models.
package modelingtest

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/inspect"
	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/modeling/event"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/modeling/wakeup"
)

// CheckTicking asserts that the inspector's static view of the package that
// declares the Spec type S matches a live ticking.Definition: the same
// defaults, the ports of P (with the same port groups), and the middlewares
// of M in the same order. Every component adds one test calling it (or
// CheckWakeup or CheckEvent), which turns "the static and runtime views
// agree" into a CI guarantee instead of a convention.
func CheckTicking[S, T, R, P, M any](
	t *testing.T, def ticking.Definition[S, T, R, P, M],
) {
	t.Helper()
	checkModel[S, P, M](t, schema.ModelTicking, def.DefaultSpec)
}

// CheckWakeup is CheckTicking for a wakeup.Definition.
func CheckWakeup[S, T, R, P, M any](
	t *testing.T, def wakeup.Definition[S, T, R, P, M],
) {
	t.Helper()
	checkModel[S, P, M](t, schema.ModelWakeup, def.DefaultSpec)
}

// CheckEvent is CheckTicking for an event.Definition.
func CheckEvent[S, T, R, P, M any](
	t *testing.T, def event.Definition[S, T, R, P, M],
) {
	t.Helper()
	checkModel[S, P, M](t, schema.ModelEvent, def.DefaultSpec)
}

func checkModel[S, P, M any](t *testing.T, model string, defaultSpec S) {
	t.Helper()

	pkgPath := reflect.TypeFor[S]().PkgPath()
	if pkgPath == "" {
		t.Fatalf("Spec type %s must be a named type declared in the "+
			"component package", reflect.TypeFor[S]())
	}

	static := staticDefinition(t, pkgPath)

	if static.Model != model {
		t.Errorf("model: static %q, runtime %q", static.Model, model)
	}

	checkDefaults(t, static, defaultSpec)

	checkNames(t, "ports", staticPortNames(static.Ports), runtimePortNames[P]())
	checkNames(t, "middlewares",
		staticMiddlewareNames(static.Middlewares), fieldNames(reflect.TypeFor[M]()))
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

// checkDefaults compares Go field values, before encoding/json applies string
// tags or omitempty. This preserves integer precision.
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

// defaultValue uses the inspector's representation of scalar values: 64-bit
// integers and floats, so comparisons stay exact.
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
	default:
		return nil
	}
}

func checkNames(t *testing.T, what string, static, runtime []string) {
	t.Helper()

	if !reflect.DeepEqual(static, runtime) {
		t.Errorf("%s: static %v, runtime %v", what, static, runtime)
	}
}

// staticPortNames lists port names, with groups written "Name[]".
func staticPortNames(ports []schema.Port) []string {
	out := make([]string, len(ports))
	for i, p := range ports {
		out[i] = p.Name
		if p.Group {
			out[i] += "[]"
		}
	}

	return out
}

// runtimePortNames lists the fields of P, with slice fields (port groups)
// written "Name[]".
func runtimePortNames[P any]() []string {
	t := reflect.TypeFor[P]()

	out := make([]string, t.NumField())
	for i := range out {
		f := t.Field(i)
		out[i] = f.Name
		if f.Type.Kind() == reflect.Slice {
			out[i] += "[]"
		}
	}

	return out
}

func staticMiddlewareNames(mws []schema.Middleware) []string {
	out := make([]string, len(mws))
	for i, mw := range mws {
		out[i] = mw.Name
	}

	return out
}

func fieldNames(t reflect.Type) []string {
	out := make([]string, t.NumField())
	for i := range out {
		out[i] = t.Field(i).Name
	}

	return out
}
