package modeling

import (
	"reflect"
	"sync"

	"github.com/sarchlab/akita/v5/sim"
)

// Component is a generic component that combines Spec, State, Ports, and
// Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Component uses A-B double buffering for state: 'current' is the read-only
// state visible via [GetState], and 'next' is the writable buffer for the
// upcoming tick. During [Tick], current is deep-copied into next before the
// middleware pipeline runs; after the pipeline completes, next becomes current.
//
// Component embeds [sim.TickingComponent] for tick-based lifecycle management
// and [sim.MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any] struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	spec    S
	current T
	next    T
}

// GetSpec returns the immutable specification.
func (c *Component[S, T]) GetSpec() S {
	return c.spec
}

// GetState returns the current (A-buffer) state. This is the read-only
// snapshot for the current tick.
func (c *Component[S, T]) GetState() T {
	return c.current
}

// GetNextState returns a pointer to the next (B-buffer) state, allowing
// direct mutation of the state that will become current after the tick.
func (c *Component[S, T]) GetNextState() *T {
	return &c.next
}

// SetNextState sets the next (B-buffer) state directly.
func (c *Component[S, T]) SetNextState(state T) {
	c.next = state
}

// SetState sets both current and next buffers. This is used for
// initialization and save/load scenarios where both buffers must agree.
func (c *Component[S, T]) SetState(state T) {
	c.current = state
	c.next = deepCopy(state)
}

// Tick performs the double-buffer cycle:
//  1. Deep-copy current into next.
//  2. Run the middleware pipeline (which may modify next via GetNextState/SetNextState or SetState).
//  3. Swap: current = next.
func (c *Component[S, T]) Tick() bool {
	c.next = deepCopy(c.current)
	madeProgress := c.MiddlewareHolder.Tick()
	c.current = c.next

	return madeProgress
}

// CommitTick promotes next to current without a deep copy. This is used by
// components that implement their own Tick method with a custom copy strategy.
func (c *Component[S, T]) CommitTick() {
	c.current = c.next
}

// ResetTick resets the TickScheduler so that future TickLater calls can
// schedule new events. This is used after loading state from a checkpoint.
func (c *Component[S, T]) ResetTick() {
	c.TickScheduler.Reset()
}

// ResetAndRestartTick resets the TickScheduler and schedules a new tick.
// This is used after loading state from a checkpoint when the component
// needs to immediately resume ticking.
func (c *Component[S, T]) ResetAndRestartTick() {
	c.TickScheduler.Reset()
	c.TickLater()
}

// deepCopy creates a deep copy of a value using reflection-based traversal.
// It handles structs, slices, maps, arrays, and primitive types. It panics
// on pointer, interface, channel, or func types, which should never appear
// in State structs per the modeling spec.
func deepCopy[T any](src T) T {
	srcVal := reflect.ValueOf(&src).Elem()
	dstVal := reflectDeepCopy(srcVal)
	return dstVal.Interface().(T)
}

// typeNeedsDeepCopy reports whether a type contains slices or maps that
// require deep copying. Types that are purely value types (primitives,
// strings, arrays of primitives, structs of primitives) can be copied
// with a simple Set/Copy. This result is cached in a sync.Map for
// performance.
var typeNeedsDeepCopyCache sync.Map // map[reflect.Type]bool

func typeNeedsDeepCopy(t reflect.Type) bool {
	if cached, ok := typeNeedsDeepCopyCache.Load(t); ok {
		return cached.(bool)
	}

	// Temporarily store false to handle recursive types (shouldn't happen
	// per spec, but guards against infinite loops).
	typeNeedsDeepCopyCache.Store(t, false)

	result := computeNeedsDeepCopy(t)
	typeNeedsDeepCopyCache.Store(t, result)

	return result
}

func computeNeedsDeepCopy(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Map:
		return true
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if typeNeedsDeepCopy(t.Field(i).Type) {
				return true
			}
		}
		return false
	case reflect.Array:
		return typeNeedsDeepCopy(t.Elem())
	default:
		return false
	}
}

func reflectDeepCopy(src reflect.Value) reflect.Value {
	switch src.Kind() {
	case reflect.Struct:
		dst := reflect.New(src.Type()).Elem()
		numFields := src.NumField()
		for i := 0; i < numFields; i++ {
			sf := src.Field(i)
			if typeNeedsDeepCopy(sf.Type()) {
				dst.Field(i).Set(reflectDeepCopy(sf))
			} else {
				dst.Field(i).Set(sf)
			}
		}
		return dst
	case reflect.Slice:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		n := src.Len()
		dst := reflect.MakeSlice(src.Type(), n, n)
		if n == 0 {
			return dst
		}
		if !typeNeedsDeepCopy(src.Type().Elem()) {
			reflect.Copy(dst, src)
		} else {
			for i := 0; i < n; i++ {
				dst.Index(i).Set(reflectDeepCopy(src.Index(i)))
			}
		}
		return dst
	case reflect.Map:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeMapWithSize(src.Type(), src.Len())
		iter := src.MapRange()
		keyNeedsCopy := typeNeedsDeepCopy(src.Type().Key())
		valNeedsCopy := typeNeedsDeepCopy(src.Type().Elem())
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()
			if keyNeedsCopy {
				k = reflectDeepCopy(k)
			}
			if valNeedsCopy {
				v = reflectDeepCopy(v)
			}
			dst.SetMapIndex(k, v)
		}
		return dst
	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		if !typeNeedsDeepCopy(src.Type().Elem()) {
			dst.Set(src) // value copy for arrays of primitives
		} else {
			for i := 0; i < src.Len(); i++ {
				dst.Index(i).Set(reflectDeepCopy(src.Index(i)))
			}
		}
		return dst
	default:
		// Primitive types (int, uint, float, bool, string, etc.)
		return src
	}
}
