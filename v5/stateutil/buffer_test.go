package stateutil

import (
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check: *Buffer[int] must satisfy queueing.Buffer.
var _ queueing.Buffer = (*Buffer[int])(nil)

func TestBufferName(t *testing.T) {
	b := &Buffer[int]{BufferName: "test_buf", Cap: 4}
	assert.Equal(t, "test_buf", b.Name())
}

func TestBufferCapacity(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 8}
	assert.Equal(t, 8, b.Capacity())
}

func TestBufferPushPopSize(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 3}
	assert.Equal(t, 0, b.Size())

	b.Push(10)
	b.Push(20)
	assert.Equal(t, 2, b.Size())

	v := b.Pop()
	assert.Equal(t, 10, v)
	assert.Equal(t, 1, b.Size())

	v = b.Pop()
	assert.Equal(t, 20, v)
	assert.Equal(t, 0, b.Size())
}

func TestBufferPopEmpty(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 2}
	v := b.Pop()
	assert.Nil(t, v)
}

func TestBufferPeek(t *testing.T) {
	b := &Buffer[string]{BufferName: "b", Cap: 2}

	v := b.Peek()
	assert.Nil(t, v)

	b.Push("hello")
	b.Push("world")
	v = b.Peek()
	assert.Equal(t, "hello", v)
	assert.Equal(t, 2, b.Size()) // Peek should not remove.
}

func TestBufferCanPush(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 2}
	assert.True(t, b.CanPush())

	b.Push(1)
	assert.True(t, b.CanPush())

	b.Push(2)
	assert.False(t, b.CanPush())
}

func TestBufferPushOverflowPanics(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 1}
	b.Push(1)
	assert.Panics(t, func() { b.Push(2) })
}

func TestBufferClear(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 5}
	b.Push(1)
	b.Push(2)
	b.Push(3)

	b.Clear()
	assert.Equal(t, 0, b.Size())
	assert.True(t, b.CanPush())
	assert.Nil(t, b.Pop())
}

func TestBufferPushTypedPopTyped(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 3}

	b.PushTyped(42)
	b.PushTyped(99)

	v, ok := b.PopTyped()
	assert.True(t, ok)
	assert.Equal(t, 42, v)

	v, ok = b.PopTyped()
	assert.True(t, ok)
	assert.Equal(t, 99, v)

	v, ok = b.PopTyped()
	assert.False(t, ok)
	assert.Equal(t, 0, v) // zero value
}

func TestBufferPushTypedOverflowPanics(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 1}
	b.PushTyped(1)
	assert.Panics(t, func() { b.PushTyped(2) })
}

func TestBufferJSONRoundTrip(t *testing.T) {
	b := &Buffer[int]{BufferName: "my_buf", Cap: 4}
	b.PushTyped(10)
	b.PushTyped(20)
	b.PushTyped(30)

	data, err := json.Marshal(b)
	require.NoError(t, err)

	var b2 Buffer[int]
	err = json.Unmarshal(data, &b2)
	require.NoError(t, err)

	assert.Equal(t, "my_buf", b2.BufferName)
	assert.Equal(t, 4, b2.Cap)
	assert.Equal(t, []int{10, 20, 30}, b2.Elements)
	assert.Equal(t, 3, b2.Size())
}

type testItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestBufferJSONRoundTripStruct(t *testing.T) {
	b := &Buffer[testItem]{BufferName: "items", Cap: 2}
	b.PushTyped(testItem{ID: 1, Name: "foo"})
	b.PushTyped(testItem{ID: 2, Name: "bar"})

	data, err := json.Marshal(b)
	require.NoError(t, err)

	var b2 Buffer[testItem]
	err = json.Unmarshal(data, &b2)
	require.NoError(t, err)

	assert.Equal(t, 2, b2.Size())
	v, ok := b2.PopTyped()
	assert.True(t, ok)
	assert.Equal(t, testItem{ID: 1, Name: "foo"}, v)
}

func TestBufferFIFOOrder(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 5}
	for i := 0; i < 5; i++ {
		b.PushTyped(i)
	}
	for i := 0; i < 5; i++ {
		v, ok := b.PopTyped()
		assert.True(t, ok)
		assert.Equal(t, i, v)
	}
}
