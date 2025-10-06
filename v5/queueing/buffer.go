// Package queueingv5 provides buffer and pipeline implementations for the Akita simulation framework.
package queueingv5

import (
	"log"

	"github.com/sarchlab/akita/v4/sim"
)

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &sim.HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &sim.HookPos{Name: "Buf Pop"}

// Buffer is a FIFO queue for anything.
type Buffer struct {
	sim.HookableBase

	name     string
	capacity int
	elements []interface{}
}

// NewBuffer creates a new buffer object.
func NewBuffer(name string, capacity int) *Buffer {
	sim.NameMustBeValid(name)

	return &Buffer{
		name:     name,
		capacity: capacity,
	}
}

// Name returns the name of the buffer.
func (b *Buffer) Name() string {
	return b.name
}

// CanPush checks if the buffer can accept a new element.
func (b *Buffer) CanPush() bool {
	return len(b.elements) < b.capacity
}

// Push adds an element to the buffer.
func (b *Buffer) Push(e interface{}) {
	if len(b.elements) >= b.capacity {
		log.Panic("buffer overflow")
	}

	b.elements = append(b.elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}
}

// Pop removes and returns the first element from the buffer.
func (b *Buffer) Pop() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	e := b.elements[0]
	b.elements = b.elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    HookPosBufPop,
			Item:   e,
			Detail: nil,
		})
	}

	return e
}

// Peek returns the first element from the buffer without removing it.
func (b *Buffer) Peek() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	return b.elements[0]
}

// Capacity returns the maximum capacity of the buffer.
func (b *Buffer) Capacity() int {
	return b.capacity
}

// Size returns the current number of elements in the buffer.
func (b *Buffer) Size() int {
	return len(b.elements)
}

// Clear removes all elements from the buffer.
func (b *Buffer) Clear() {
	b.elements = nil
}