package stateutil

import (
	"log"

	"github.com/sarchlab/akita/v5/sim"
)

// Buffer is a generic FIFO queue that satisfies the queueing.Buffer interface.
// It is JSON-serializable via exported fields with json tags.
type Buffer[T any] struct {
	sim.HookableBase `json:"-"`

	BufferName string `json:"buffer_name"`
	Cap        int    `json:"cap"`
	Elements   []T    `json:"elements"`
}

// Name returns the name of the buffer.
func (b *Buffer[T]) Name() string {
	return b.BufferName
}

// Capacity returns the capacity of the buffer.
func (b *Buffer[T]) Capacity() int {
	return b.Cap
}

// Size returns the number of elements currently in the buffer.
func (b *Buffer[T]) Size() int {
	return len(b.Elements)
}

// CanPush returns true if the buffer has room for another element.
func (b *Buffer[T]) CanPush() bool {
	return len(b.Elements) < b.Cap
}

// Push adds an element to the back of the buffer. It panics if the buffer
// is already at capacity. The element is accepted as interface{} to satisfy
// the queueing.Buffer interface.
func (b *Buffer[T]) Push(e interface{}) {
	if len(b.Elements) >= b.Cap {
		log.Panic("buffer overflow")
	}

	typed := e.(T)
	b.Elements = append(b.Elements, typed)

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
		})
	}
}

// Pop removes and returns the front element from the buffer. Returns nil if
// the buffer is empty.
func (b *Buffer[T]) Pop() interface{} {
	if len(b.Elements) == 0 {
		return nil
	}

	e := b.Elements[0]
	b.Elements = b.Elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPop,
			Item:   e,
		})
	}

	return e
}

// Peek returns the front element without removing it. Returns nil if the
// buffer is empty.
func (b *Buffer[T]) Peek() interface{} {
	if len(b.Elements) == 0 {
		return nil
	}

	return b.Elements[0]
}

// Clear removes all elements from the buffer.
func (b *Buffer[T]) Clear() {
	b.Elements = nil
}

// PushTyped adds a typed element to the back of the buffer. It panics if
// the buffer is already at capacity.
func (b *Buffer[T]) PushTyped(e T) {
	if len(b.Elements) >= b.Cap {
		log.Panic("buffer overflow")
	}

	b.Elements = append(b.Elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
		})
	}
}

// PopTyped removes and returns the front element with its concrete type.
// The second return value is false if the buffer is empty.
func (b *Buffer[T]) PopTyped() (T, bool) {
	if len(b.Elements) == 0 {
		var zero T
		return zero, false
	}

	e := b.Elements[0]
	b.Elements = b.Elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPop,
			Item:   e,
		})
	}

	return e, true
}

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &sim.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &sim.HookPos{Name: "Buffer Pop"}
