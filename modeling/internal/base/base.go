// Package base holds what the three component models (modeling/ticking,
// modeling/wakeup, and modeling/event) share: the ComponentBase each model's
// Component embeds, the steps of Build, and the dispatch of events to
// middlewares. It is internal because a component model is defined only by
// those three packages.
package base

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/internal/portwalk"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// ComponentBase is what every component model shares: an instance's five
// structs — Spec, State, Resources, Ports, and Middlewares — and the methods
// that do not depend on how the instance is scheduled. The Component of each
// model embeds it and adds its scheduling.
type ComponentBase[S, T, R, P, M any] struct {
	hooking.HookableBase

	// State is the instance's mutable runtime data. Only the component itself
	// (its NewState and middlewares) writes it.
	State T

	// Ports holds the instance's ports: the port instances the system builder
	// passed to Build, bound to this instance. They are fixed from then on.
	Ports P

	// Middlewares holds the instance's behavior. Every event the instance
	// receives goes to each field in declaration order.
	Middlewares M

	name       string
	owner      modeling.Component
	simulation timing.Simulation
	spec       S
	resources  R
}

// Init sets up the ComponentBase embedded in owner, a component being built:
// it records the instance's name, simulation, Spec, and Resources, and binds
// each port to owner. A model's Build calls it first, then creates the State
// and the middlewares, and calls Register last.
func Init[S, T, R, P, M any](
	base *ComponentBase[S, T, R, P, M],
	owner modeling.Component,
	sim timing.Simulation,
	name string,
	spec S,
	resources R,
	ports P,
) {
	base.name = name
	base.owner = owner
	base.simulation = sim
	base.spec = cloneSpec(spec)
	base.resources = resources
	base.Ports = ports
	bindPorts(owner, &base.Ports)
}

// Register registers the instance that embeds base with its simulation: each
// of its ports, then the instance as the handler of its events (when the
// engine takes handlers) and as a component. A model's Build calls it last,
// once the State and the middlewares exist, so a Build that fails registers
// nothing.
func Register[S, T, R, P, M any](base *ComponentBase[S, T, R, P, M]) {
	registerPorts(base.simulation, &base.Ports)

	if handlers, ok := base.simulation.GetEngine().(timing.HandlerRegistry); ok {
		handlers.RegisterHandler(base.name, base.owner)
	}

	base.simulation.RegisterComponent(base.owner)
}

// Name returns the instance name given to Build.
func (c *ComponentBase[S, T, R, P, M]) Name() string {
	return c.name
}

// TypeName returns the component type's name: the import path of the package
// that declares it.
func (c *ComponentBase[S, T, R, P, M]) TypeName() string {
	return reflect.TypeFor[S]().PkgPath()
}

// NewID allocates an ID, unique within the instance's simulation, for a
// message or event the instance creates.
func (c *ComponentBase[S, T, R, P, M]) NewID() uint64 {
	return c.simulation.NewID()
}

// CurrentTime returns the simulation's current time.
func (c *ComponentBase[S, T, R, P, M]) CurrentTime() timing.VTimeInPicoSec {
	return c.simulation.GetEngine().CurrentTime()
}

// Spec returns the instance's configuration. The returned value is a copy, but
// its slices are the instance's own: treat them as read-only.
func (c *ComponentBase[S, T, R, P, M]) Spec() S {
	return c.spec
}

// Resources returns the instance's shared-resource references.
func (c *ComponentBase[S, T, R, P, M]) Resources() R {
	return c.resources
}

var middlewareType = reflect.TypeFor[modeling.Middleware]()

// Dispatch passes an event to each middleware in order and reports whether
// any of them made progress. Each model's Component calls it from Handle.
func Dispatch(middlewares []modeling.Middleware, e timing.Event) bool {
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
// modeling.Middleware, and is set.
func OrderedMiddlewares(middlewares any) []modeling.Middleware {
	v := reflect.Indirect(reflect.ValueOf(middlewares))

	t := v.Type()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("modeling: Middlewares type %s must be a struct", t))
	}

	out := make([]modeling.Middleware, 0, t.NumField())
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

		out = append(out, fv.Interface().(modeling.Middleware))
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

// bindPorts binds every port in the Ports struct that ports points to to
// owner. Every port must be given and carry its full name: "<owner>.<field>"
// for a port, and "<owner>.<field>[i]" for member i of a group.
func bindPorts(owner modeling.Component, ports any) {
	portwalk.ForEach(ports, func(slot string, v reflect.Value) {
		if v.IsNil() {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is not given; pass it with WithPorts",
				owner.Name(), slot))
		}

		port := v.Interface().(messaging.Port)

		if want := owner.Name() + "." + slot; port.Name() != want {
			panic(fmt.Sprintf(
				"modeling: component %q: port %s is named %q, want %q",
				owner.Name(), slot, port.Name(), want))
		}

		if other := port.Owner(); other != nil && other != owner {
			panic(fmt.Sprintf(
				"modeling: component %q: port %q already belongs to %s",
				owner.Name(), port.Name(), ownerName(other)))
		}

		port.SetOwner(owner)
	})
}

// ownerName names a port's owner for an error message. An owner need not be
// named.
func ownerName(owner messaging.PortOwner) string {
	if named, ok := owner.(naming.Named); ok {
		return fmt.Sprintf("%q", named.Name())
	}

	return fmt.Sprintf("an unnamed %T", owner)
}

// registerPorts registers every port in the Ports struct that ports points to
// with the simulation.
func registerPorts(sim timing.Simulation, ports any) {
	portwalk.ForEach(ports, func(_ string, v reflect.Value) {
		sim.RegisterPort(v.Interface().(messaging.Port))
	})
}

// cloneSpec returns spec with its own copy of every slice field, so an
// instance shares no slice with the Definition's DefaultSpec or with the value
// given to WithSpec. A Spec holds only scalars and slices of scalars, so
// copying the slices copies everything.
func cloneSpec[S any](spec S) S {
	v := reflect.ValueOf(&spec).Elem()
	if v.Kind() != reflect.Struct {
		return spec
	}

	for i := range v.NumField() {
		f := v.Field(i)
		if f.Kind() != reflect.Slice || f.IsNil() || !f.CanSet() {
			continue
		}

		c := reflect.MakeSlice(f.Type(), f.Len(), f.Len())
		reflect.Copy(c, f)
		f.Set(c)
	}

	return spec
}
