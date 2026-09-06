package queueing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBufferName(t *testing.T) {
	b := NewBuffer[int]("test_buf", 4)
	assert.Equal(t, "test_buf", b.Name())
}

func TestBufferCapacity(t *testing.T) {
	b := NewBuffer[int]("b", 8)
	assert.Equal(t, 8, b.Capacity())
}

func TestBufferPushSize(t *testing.T) {
	b := NewBuffer[int]("b", 3)
	assert.Equal(t, 0, b.Size())

	b.Push(10)
	b.Push(20)
	assert.Equal(t, 2, b.Size())

	value0, present0 := b.Peek()
	assert.True(t, present0)
	assert.Equal(t, 10, value0)
}

func TestBufferCanPush(t *testing.T) {
	b := NewBuffer[int]("b", 2)
	assert.True(t, b.CanPush())

	b.Push(1)
	assert.True(t, b.CanPush())

	b.Push(2)
	assert.False(t, b.CanPush())
}

func TestBufferPushOverflowPanics(t *testing.T) {
	b := NewBuffer[int]("b", 1)
	b.Push(1)
	assert.Panics(t, func() { b.Push(2) })
}

func TestBufferClear(t *testing.T) {
	b := NewBuffer[int]("b", 5)
	b.Push(1)
	b.Push(2)
	b.Push(3)

	b.Clear()
	assert.Equal(t, 0, b.Size())
	assert.True(t, b.CanPush())
}

func TestBufferPeekEmpty(t *testing.T) {
	b := NewBuffer[int]("b", 3)
	value1, present1 := b.Peek()
	assert.False(t, present1)
	assert.Equal(t, 0, value1)
}

func TestBufferPopEmpty(t *testing.T) {
	b := NewBuffer[int]("b", 3)
	value2, present2 := b.Pop()
	assert.False(t, present2)
	assert.Equal(t, 0, value2)
}

func TestBufferFIFOOrder(t *testing.T) {
	b := NewBuffer[int]("b", 5)
	for i := 0; i < 5; i++ {
		b.Push(i)
	}
	for i := 0; i < 5; i++ {
		value3, present3 := b.Peek()
		assert.True(t, present3)
		assert.Equal(t, i, value3)
		value4, present4 := b.Pop()
		assert.True(t, present4)
		assert.Equal(t, i, value4)
	}
	assert.Equal(t, 0, b.Size())
}
