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

	b.PushTyped(10)
	b.PushTyped(20)
	assert.Equal(t, 2, b.Size())

	assert.Equal(t, 10, b.Peek())
}

func TestBufferCanPush(t *testing.T) {
	b := NewBuffer[int]("b", 2)
	assert.True(t, b.CanPush())

	b.PushTyped(1)
	assert.True(t, b.CanPush())

	b.PushTyped(2)
	assert.False(t, b.CanPush())
}

func TestBufferPushOverflowPanics(t *testing.T) {
	b := NewBuffer[int]("b", 1)
	b.PushTyped(1)
	assert.Panics(t, func() { b.PushTyped(2) })
}

func TestBufferClear(t *testing.T) {
	b := NewBuffer[int]("b", 5)
	b.PushTyped(1)
	b.PushTyped(2)
	b.PushTyped(3)

	b.Clear()
	assert.Equal(t, 0, b.Size())
	assert.True(t, b.CanPush())
}

func TestBufferPeekEmpty(t *testing.T) {
	b := NewBuffer[int]("b", 3)
	assert.Equal(t, 0, b.Peek())
}

func TestBufferPopEmpty(t *testing.T) {
	b := NewBuffer[int]("b", 3)
	assert.Equal(t, 0, b.Pop())
}

func TestBufferFIFOOrder(t *testing.T) {
	b := NewBuffer[int]("b", 5)
	for i := 0; i < 5; i++ {
		b.PushTyped(i)
	}
	for i := 0; i < 5; i++ {
		assert.Equal(t, i, b.Peek())
		assert.Equal(t, i, b.Pop())
	}
	assert.Equal(t, 0, b.Size())
}
