package sim

import "log"

// portBuffer is a simple FIFO buffer used internally by ports.
// It does not support hooks or naming.
type portBuffer struct {
	capacity int
	elements []Msg
}

func newPortBuffer(capacity int) *portBuffer {
	return &portBuffer{capacity: capacity}
}

func (b *portBuffer) CanPush() bool {
	return len(b.elements) < b.capacity
}

func (b *portBuffer) Push(e Msg) {
	if len(b.elements) >= b.capacity {
		log.Panic("buffer overflow")
	}
	b.elements = append(b.elements, e)
}

func (b *portBuffer) Pop() Msg {
	if len(b.elements) == 0 {
		return nil
	}
	e := b.elements[0]
	b.elements = b.elements[1:]
	return e
}

func (b *portBuffer) Peek() Msg {
	if len(b.elements) == 0 {
		return nil
	}
	return b.elements[0]
}

func (b *portBuffer) Capacity() int {
	return b.capacity
}

func (b *portBuffer) Size() int {
	return len(b.elements)
}
