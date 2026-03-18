package queueing

import (
	"log"

	"github.com/sarchlab/akita/v5/sim"
)

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &sim.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &sim.HookPos{Name: "Buffer Pop"}

// BufferState is a minimal interface for buffer inspection. It is satisfied
// by *Buffer[T] and can be used by analysis, monitoring, and arbitration
// packages that need to query buffer metadata without knowing the element type.
type BufferState interface {
	sim.Named
	sim.Hookable
	CanPush() bool
	Capacity() int
	Size() int
}

// Buffer is a generic FIFO queue. It is JSON-serializable via exported fields
// with json tags.
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
// is already at capacity. The element is accepted as interface{}.
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
