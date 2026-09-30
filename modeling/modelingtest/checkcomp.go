package modelingtest

import (
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/modeling"
)

// CheckComponent asserts that the inspector's static view of the package that
// declares the Spec type S matches a live modeling.Definition: the same name
// and defaults, the ports of P (with the same port groups), and the
// middlewares of M in the same order.
//
// CheckComponent is the prototype counterpart of CheckDefinition for the #492
// component model.
func CheckComponent[S, T, R, P, M any](
	t *testing.T, def modeling.Definition[S, T, R, P, M],
) {
	t.Helper()

	pkgPath := reflect.TypeFor[S]().PkgPath()
	if pkgPath == "" {
		t.Fatalf("Spec type %s must be a named type declared in the "+
			"component package", reflect.TypeFor[S]())
	}

	static := staticDefinition(t, pkgPath)

	if static.Name != def.Name {
		t.Errorf("name: static %q, runtime %q", static.Name, def.Name)
	}

	checkDefaults(t, static, def.DefaultSpec)

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
