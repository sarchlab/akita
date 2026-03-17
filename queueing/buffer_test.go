package queueing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferName(t *testing.T) {
	b := &Buffer[int]{BufferName: "test_buf", Cap: 4}
	assert.Equal(t, "test_buf", b.Name())
}

func TestBufferCapacity(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 8}
	assert.Equal(t, 8, b.Capacity())
}

func TestBufferPushSize(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 3}
	assert.Equal(t, 0, b.Size())

	b.Push(10)
	b.Push(20)
	assert.Equal(t, 2, b.Size())

	// Verify elements directly
	assert.Equal(t, 10, b.Elements[0])
	assert.Equal(t, 20, b.Elements[1])
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
}

func TestBufferPushTyped(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 3}

	b.PushTyped(42)
	b.PushTyped(99)

	assert.Equal(t, 2, b.Size())
	assert.Equal(t, 42, b.Elements[0])
	assert.Equal(t, 99, b.Elements[1])
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
	assert.Equal(t, testItem{ID: 1, Name: "foo"}, b2.Elements[0])
}

func TestBufferFIFOOrder(t *testing.T) {
	b := &Buffer[int]{BufferName: "b", Cap: 5}
	for i := 0; i < 5; i++ {
		b.PushTyped(i)
	}
	for i := 0; i < 5; i++ {
		assert.Equal(t, i, b.Elements[i])
	}
}

// Compile-time check: *Buffer[int] must satisfy BufferState.
var _ BufferState = (*Buffer[int])(nil)
