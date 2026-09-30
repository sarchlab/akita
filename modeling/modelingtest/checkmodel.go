package modelingtest

import (
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/modeling/event"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/modeling/wakeup"
)

// CheckTicking asserts that the inspector's static view of the package that
// declares the Spec type S matches a live ticking.Definition: the same
// defaults, the ports of P (with the same port groups), and the middlewares
// of M in the same order.
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
