// Package base holds what the three component models (modeling/ticking,
// modeling/wakeup, and modeling/event) share: the ComponentBase each model's
// Component embeds, the steps of Build, and the dispatch of events to
// middlewares. It is internal because a component model is defined only by
// those three packages.
package base

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/internal/portwalk"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ComponentBase is what every component model shares: an instance's five
// structs — Spec, State, Resources, Ports, and Middlewares — and the methods
// that do not depend on how the instance is scheduled. The Component of each
// model embeds it and adds its scheduling.
type ComponentBase[S, T, R, P, M any] struct {
	hooking.HookableBase

	// Spec is the instance's configuration, given to Build. It is fixed from
	// then on; Build copies its slices, so the instance shares none with
	// DefaultSpec.
	Spec S

	// State is the instance's mutable runtime data. Only the component itself
	// (its NewState and middlewares) writes it.
	State T

	// Resources holds the instance's references to shared objects, given to
	// Build. They are fixed from then on.
	Resources R

	// Ports holds the slots assigned through BindPort during wiring.
	// Initialization freezes these bindings; do not assign the fields directly.
	Ports P

	// Middlewares holds the instance's behavior. Every event the instance
	// receives goes to each field in declaration order.
	Middlewares M

	name        string
	owner       modeling.Component
	simulation  timing.Simulation
	initialize  func()
	initialized bool
}

// Init records the instance identity, simulation, Spec and Resources.
// State and Middlewares are created later by the deferred initializer.
func Init[S, T, R, P, M any](
	base *ComponentBase[S, T, R, P, M],
	owner modeling.Component,
	sim timing.Simulation,
	name string,
	spec S,
	resources R,
) {
	base.name = name
	base.owner = owner
	base.simulation = sim
	base.Spec = cloneSpec(spec)
	base.Resources = resources
}

// Register adds the configured instance and its event handler to the simulation.
// BindPort registers ports separately during wiring.
func Register[S, T, R, P, M any](base *ComponentBase[S, T, R, P, M]) {
	base.simulation.Engine().RegisterHandler(base.name, base.owner)

	base.simulation.RegisterComponent(base.owner)
}

// Name returns the instance name given to Build.
func (c *ComponentBase[S, T, R, P, M]) Name() string {
	return c.name
}

// NewID allocates an ID, unique within the instance's simulation, for a
// message or event the instance creates.
func (c *ComponentBase[S, T, R, P, M]) NewID() uint64 {
	return c.simulation.NewID()
}

// CurrentTime returns the simulation's current time.
func (c *ComponentBase[S, T, R, P, M]) CurrentTime() timing.VTimeInPicoSec {
	return c.simulation.Engine().CurrentTime()
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

// SetInitializer records deferred state and middleware construction.
func SetInitializer[S, T, R, P, M any](c *ComponentBase[S, T, R, P, M], fn func()) { c.initialize = fn }

// RequireSetup checks the simulation's assembly phase.
func (c *ComponentBase[S, T, R, P, M]) RequireSetup() { c.simulation.RequireSetup() }

// BindPort assigns one declared port slot and establishes its owner and name.
// Indexed slice slots (for example Inputs[2]) grow the group as needed.
func (c *ComponentBase[S, T, R, P, M]) BindPort(slot string, port messaging.Port) {
	c.RequireSetup()
	if port == nil || (reflect.ValueOf(port).Kind() == reflect.Pointer && reflect.ValueOf(port).IsNil()) {
		panic("modeling: cannot bind a nil port")
	}
	if port.Owner() != nil {
		panic("modeling: port already has an owner")
	}
	name := c.name + "." + slot
	naming.MustBeValid(name)
	c.simulation.RequireNameAvailable(name)
	fieldName := slot
	index := -1
	if i := strings.IndexByte(slot, '['); i >= 0 {
		if !strings.HasSuffix(slot, "]") {
			panic("modeling: invalid port slot " + slot)
		}
		fieldName = slot[:i]
		var err error
		index, err = strconv.Atoi(slot[i+1 : len(slot)-1])
		if err != nil || index < 0 || fmt.Sprintf("%s[%d]", fieldName, index) != slot {
			panic("modeling: invalid port index " + slot)
		}
	}
	field := reflect.ValueOf(&c.Ports).Elem().FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() {
		panic("modeling: unknown port slot " + slot)
	}
	portType := reflect.TypeFor[messaging.Port]()
	if index < 0 {
		if field.Type() != portType {
			panic("modeling: port group requires an index: " + slot)
		}
		if !field.IsNil() {
			panic("modeling: port slot already bound: " + slot)
		}
	} else {
		if field.Type() != reflect.SliceOf(portType) {
			panic("modeling: slot is not a port group: " + slot)
		}
		if index < field.Len() && !field.Index(index).IsNil() {
			panic("modeling: port slot already bound: " + slot)
		}
	}
	port.BindOwner(c.owner, name)
	if index < 0 {
		field.Set(reflect.ValueOf(port))
	} else {
		if index >= field.Len() {
			grown := reflect.MakeSlice(field.Type(), index+1, index+1)
			reflect.Copy(grown, field)
			field.Set(grown)
		}
		field.Index(index).Set(reflect.ValueOf(port))
	}
	c.simulation.RegisterPort(port)
}

// ValidateSetup checks all declared port slots before any initializer runs.
func (c *ComponentBase[S, T, R, P, M]) ValidateSetup() error {
	var failures []error
	portwalk.ForEach(&c.Ports, func(slot string, v reflect.Value) {
		if v.IsNil() {
			failures = append(failures, fmt.Errorf("port %s is not bound", slot))
			return
		}
		p := v.Interface().(messaging.Port)
		if reflect.ValueOf(p).Kind() == reflect.Pointer && reflect.ValueOf(p).IsNil() {
			failures = append(failures, fmt.Errorf("port %s is nil", slot))
			return
		}
		if p.Owner() != c.owner || p.Name() != c.name+"."+slot {
			failures = append(failures, fmt.Errorf("port %s has inconsistent ownership", slot))
		}
	})
	return errors.Join(failures...)
}

// InitializeComponent is called by the simulation after all setup validates.
func (c *ComponentBase[S, T, R, P, M]) InitializeComponent() {
	if c.initialized {
		panic("modeling: component already initialized")
	}
	if c.initialize != nil {
		c.initialize()
	}
	c.initialized = true
}

// RequireInitialized rejects execution or snapshots before initialization.
func (c *ComponentBase[S, T, R, P, M]) RequireInitialized() {
	if !c.initialized {
		panic("modeling: component is not initialized")
	}
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
