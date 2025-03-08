package sim

import "log"

// HookPosBufPush marks when an element is pushed into the buffer.
var HookPosBufPush = &HookPos{Name: "Buffer Push"}

// HookPosBufPop marks when an element is popped from the buffer.
var HookPosBufPop = &HookPos{Name: "Buf Pop"}

// A Buffer is a fifo queue for anything
type Buffer interface {
	Named
	Hookable

	CanPush() bool
	Push(e any)
	Pop() any
	Peek() any
	Capacity() int
	Size() int

	// Remove all elements in the buffer
	Clear()
}

// NewBuffer creates a default buffer object.
func NewBuffer(name string, capacity int) Buffer {
	NameMustBeValid(name)

	b := &bufferImpl{
		name:     name,
		capacity: capacity,
		elements: make([]any, capacity),
		head:     0,
		tail:     0,
		size:     0,
	}

	return b
}

type bufferImpl struct {
	HookableBase

	name     string
	capacity int
	// Use a fixed-size slice for the circular buffer
	elements []any
	// head points to the index of the next element to pop
	head int
	// tail points to the next insertion index
	tail int
	// size is the current number of elements in the buffer
	size int
}

// Name returns the name of the buffer.
func (b *bufferImpl) Name() string {
	return b.name
}

func (b *bufferImpl) CanPush() bool {
	return b.size < b.capacity
}

func (b *bufferImpl) Push(e any) {
	if b.size == b.capacity {
		log.Panic("buffer overflow")
	}

	b.elements[b.tail] = e
	b.tail = (b.tail + 1) % b.capacity
	b.size++

	if b.NumHooks() > 0 {
		b.InvokeHook(HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}
}

func (b *bufferImpl) Pop() any {
	if b.size == 0 {
		return nil
	}

	e := b.elements[b.head]

	// Avoid memory leak by nil-ing the slot
	b.elements[b.head] = nil
	b.head = (b.head + 1) % b.capacity
	b.size--

	if b.NumHooks() > 0 {
		b.InvokeHook(HookCtx{
			Domain: b,
			Pos:    HookPosBufPop, // Use the pop hook
			Item:   e,
			Detail: nil,
		})
	}

	return e
}

func (b *bufferImpl) Peek() any {
	if b.size == 0 {
		return nil
	}

	return b.elements[b.head]
}

func (b *bufferImpl) Capacity() int {
	return b.capacity
}

func (b *bufferImpl) Size() int {
	return b.size
}

func (b *bufferImpl) Clear() {
	b.head = 0
	b.tail = 0
	b.size = 0

	for i := range b.elements {
		b.elements[i] = make([]any, b.capacity)
	}
}
