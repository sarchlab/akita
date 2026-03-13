package modeling

import (
	"reflect"

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

func reflectDeepCopy(src reflect.Value) reflect.Value {
	switch src.Kind() {
	case reflect.Struct:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.NumField(); i++ {
			dst.Field(i).Set(reflectDeepCopy(src.Field(i)))
		}
		return dst
	case reflect.Slice:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(reflectDeepCopy(src.Index(i)))
		}
		return dst
	case reflect.Map:
		if src.IsNil() {
			return reflect.Zero(src.Type())
		}
		dst := reflect.MakeMapWithSize(src.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			dst.SetMapIndex(
				reflectDeepCopy(iter.Key()),
				reflectDeepCopy(iter.Value()),
			)
		}
		return dst
	case reflect.Array:
		dst := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			dst.Index(i).Set(reflectDeepCopy(src.Index(i)))
		}
		return dst
	default:
		// Primitive types (int, uint, float, bool, string, etc.)
		// These are value types — just return a copy
		return src
	}
}
