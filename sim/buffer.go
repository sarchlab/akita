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
	GetElementByIndex(int) any // Jijie todo
	RemoveElementByIndex(int) any // Jijie todo
	GetAllElements() []any // Jijie todo


	// Remove all elements in the buffer
	Clear()
}

// NewBuffer creates a default buffer object.
func NewBuffer(name string, capacity int) Buffer {
	NameMustBeValid(name)

	return &bufferImpl{
		name:     name,
		capacity: capacity,
	}
}

type bufferImpl struct {
	HookableBase

	name     string
	capacity int
	elements []any
}

// Name returns the name of the buffer.
func (b *bufferImpl) Name() string {
	return b.name
}

func (b *bufferImpl) CanPush() bool {
	return len(b.elements) < b.capacity
}

func (b *bufferImpl) Push(e any) {
	if len(b.elements) >= b.capacity {
		log.Panic("buffer overflow")
	}

	b.elements = append(b.elements, e)

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
	if len(b.elements) == 0 {
		return nil
	}

	e := b.elements[0]
	b.elements = b.elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(HookCtx{
			Domain: b,
			Pos:    HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}

	return e
}

func (b *bufferImpl) Peek() any {
	if len(b.elements) == 0 {
		return nil
	}

	return b.elements[0]
}

func (b *bufferImpl) Capacity() int {
	return b.capacity
}

func (b *bufferImpl) Size() int {
	return len(b.elements)
}

func (b *bufferImpl) Clear() {
	b.elements = nil
}

func (b *bufferImpl) GetElementByIndex(index int) any {
	if index < 0 || index >= len(b.elements) {
        return nil
    }
	return b.elements[index]
}

func (b *bufferImpl) RemoveElementByIndex(index int) any {
    if index < 0 || index >= len(b.elements) {
        return nil
    }
    elementToRemove := b.elements[index]
    b.elements = append(b.elements[:index], b.elements[index+1:]...)
    return elementToRemove
}

// Make a copy of the elements slice to prevent modification from outside
func (b *bufferImpl) GetAllElements() []any {
    elementsCopy := make([]any, len(b.elements))
    copy(elementsCopy, b.elements)
    return elementsCopy
}