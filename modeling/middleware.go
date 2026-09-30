package modeling

import (
	"fmt"
	"reflect"
)

// Middleware defines the actions of a component.
type Middleware interface {
	// Tick processes a tick event. It returns true if progress is made.
	Tick() bool
}

var middlewareType = reflect.TypeFor[Middleware]()

// OrderedMiddlewares returns the fields of a Middlewares struct in declaration
// order, the order a component runs them. middlewares is the struct or a
// pointer to it. It panics unless every field is exported, implements
// Middleware, and is set.
func OrderedMiddlewares(middlewares any) []Middleware {
	v := reflect.Indirect(reflect.ValueOf(middlewares))

	t := v.Type()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("modeling: Middlewares type %s must be a struct", t))
	}

	out := make([]Middleware, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || !f.Type.Implements(middlewareType) {
			panic(fmt.Sprintf(
				"modeling: Middlewares field %s.%s must be exported and "+
					"implement modeling.Middleware", t, f.Name))
		}

		fv := v.Field(i)
		if isNilable(fv.Kind()) && fv.IsNil() {
			panic(fmt.Sprintf(
				"modeling: Middlewares field %s.%s is not set by NewMiddlewares",
				t, f.Name))
		}

		out = append(out, fv.Interface().(Middleware))
	}

	return out
}

func isNilable(k reflect.Kind) bool {
	switch k {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice,
		reflect.Func, reflect.Chan:
		return true
	default:
		return false
	}
}

// MiddlewareHolder can maintain a list of middleware.
type MiddlewareHolder struct {
	middlewares []Middleware
}

// AddMiddleware adds a middleware to the holder.
func (holder *MiddlewareHolder) AddMiddleware(middleware Middleware) {
	holder.middlewares = append(holder.middlewares, middleware)
}

// Middlewares returns a copy of the middleware list. The copy prevents callers
// from mutating the holder's internal slice; the middleware objects themselves
// are shared.
func (holder *MiddlewareHolder) Middlewares() []Middleware {
	middlewares := make([]Middleware, len(holder.middlewares))
	copy(middlewares, holder.middlewares)

	return middlewares
}

// Tick processes a tick event. It returns true if progress is made.
func (holder *MiddlewareHolder) Tick() bool {
	progress := false

	for _, middleware := range holder.middlewares {
		if middleware.Tick() {
			progress = true
		}
	}

	return progress
}
