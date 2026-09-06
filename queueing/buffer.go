package queueing

import (
	"log"

	"github.com/sarchlab/akita/v5/hooking"
)

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &hooking.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &hooking.HookPos{Name: "Buffer Pop"}

// Buffer is a generic, bounded buffer with FIFO and indexed reads.
// Indexed removal preserves the order of the remaining elements. Buffer is not
// synchronized; callers must serialize access. Capacity checks do not reserve space.
type Buffer[T any] struct {
	hooking.HookableBase `json:"-"`

	name     string
	cap      int
	elements []T
}

// NewBuffer creates a FIFO buffer with the given name and capacity. It returns
// a value so the buffer can be embedded directly in a component's state.
func NewBuffer[T any](name string, capacity int) Buffer[T] {
	return Buffer[T]{
		name: name,
		cap:  capacity,
	}
}

// Name returns the name of the buffer.
func (b *Buffer[T]) Name() string {
	return b.name
}

// Capacity returns the capacity of the buffer.
func (b *Buffer[T]) Capacity() int {
	return b.cap
}

// Size returns the number of elements currently in the buffer.
func (b *Buffer[T]) Size() int {
	return len(b.elements)
}

// CanPush returns true if the buffer has room for another element.
func (b *Buffer[T]) CanPush() bool {
	return len(b.elements) < b.cap
}

// Push adds an element to the back of the buffer. It panics if the buffer
// is already at capacity.
func (b *Buffer[T]) Push(e T) {
	if len(b.elements) >= b.cap {
		log.Panic("buffer overflow")
	}

	b.elements = append(b.elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
		})
	}
}

// Peek returns the head element without removing it. The boolean reports
// presence, including when the stored value is the zero value of T.
func (b *Buffer[T]) Peek() (T, bool) { return b.PeekAt(0) }

// PeekAt returns the element at index without removing it. The head has index
// zero. Out-of-range indices return the zero value of T and false.
func (b *Buffer[T]) PeekAt(index int) (T, bool) {
	if index < 0 || index >= len(b.elements) {
		var zero T
		return zero, false
	}
	return b.elements[index], true
}

// UpdateFront replaces the head element and reports whether one existed.
func (b *Buffer[T]) UpdateFront(e T) bool {
	if len(b.elements) == 0 {
		return false
	}
	b.elements[0] = e
	return true
}

// Pop removes the head element and reports whether one existed.
func (b *Buffer[T]) Pop() (T, bool) { return b.PopAt(0) }

// PopAt removes and returns the element at index, preserving the order of the
// remaining elements. Out-of-range indices return the zero value of T and false
// without mutation or hooks. Removing an entry invalidates subsequent indices.
func (b *Buffer[T]) PopAt(index int) (T, bool) {
	e, ok := b.PeekAt(index)
	if !ok {
		return e, false
	}

	var zero T
	if index == 0 {
		b.elements[0] = zero
		b.elements = b.elements[1:]
	} else {
		copy(b.elements[index:], b.elements[index+1:])
		b.elements[len(b.elements)-1] = zero
		b.elements = b.elements[:len(b.elements)-1]
	}

	if b.NumHooks() > 0 {
		b.InvokeHook(hooking.HookCtx{
			Domain: b,
			Pos:    HookPosBufPop,
			Item:   e,
		})
	}
	return e, true
}

// Clear removes all elements from the buffer.
func (b *Buffer[T]) Clear() {
	b.elements = nil
}

// Elements returns a copy of the buffer's contents, front to back. It is
// intended for inspection and checkpointing; mutating the returned slice has no
// effect on the buffer.
func (b *Buffer[T]) Elements() []T {
	out := make([]T, len(b.elements))
	copy(out, b.elements)

	return out
}

// Restore replaces the buffer's contents with the given elements (front to
// back) without firing hooks. It is used to rebuild a buffer from a checkpoint
// and panics if the elements exceed the buffer's capacity.
func (b *Buffer[T]) Restore(elements []T) {
	if len(elements) > b.cap {
		log.Panicf("buffer restore overflow: %d elements, capacity %d",
			len(elements), b.cap)
	}

	b.elements = append([]T(nil), elements...)
}
