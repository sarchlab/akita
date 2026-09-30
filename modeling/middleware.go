package modeling

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/timing"
)

// Middleware is one piece of a component's behavior. Every component model
// uses it: the component passes each event it receives to its middlewares in
// the declaration order of its Middlewares struct. A tick is a TickEvent; a
// middleware ignores the events it does not handle.
type Middleware interface {
	// Handle processes an event. It returns true if it made progress:
	// changed the State, or sent or received a message.
	Handle(e timing.Event) bool
}

var middlewareType = reflect.TypeFor[Middleware]()

// Dispatch passes an event to each middleware in order and reports whether
// any of them made progress. Each model's Component calls it from Handle.
func Dispatch(middlewares []Middleware, e timing.Event) bool {
	progress := false

	for _, mw := range middlewares {
		if mw.Handle(e) {
			progress = true
		}
	}

	return progress
}

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

// MiddlewareHolder can maintain a list of middleware. It serves the Component
// API, whose middlewares are Tickers; a component model declares its
// middlewares as a Middlewares struct instead.
type MiddlewareHolder struct {
	middlewares []Ticker
}

// AddMiddleware adds a middleware to the holder.
func (holder *MiddlewareHolder) AddMiddleware(middleware Ticker) {
	holder.middlewares = append(holder.middlewares, middleware)
}

// Middlewares returns a copy of the middleware list. The copy prevents callers
// from mutating the holder's internal slice; the middleware objects themselves
// are shared.
func (holder *MiddlewareHolder) Middlewares() []Ticker {
	middlewares := make([]Ticker, len(holder.middlewares))
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
