package queueing

import (
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/stretchr/testify/require"
)

type bufferPopRecorder struct{ items []any }

func (h *bufferPopRecorder) Func(ctx hooking.HookCtx) {
	if ctx.Pos == HookPosBufPop {
		h.items = append(h.items, ctx.Item)
	}
}

func TestBufferIndexedRemoval(t *testing.T) {
	b := NewBuffer[int]("indexed", 4)
	h := &bufferPopRecorder{}
	b.AcceptHook(h)
	for _, v := range []int{0, 10, 20, 30} {
		b.Push(v)
	}
	for _, i := range []int{-1, 4, 100} {
		v, ok := b.PeekAt(i)
		require.Zero(t, v)
		require.False(t, ok)
		v, ok = b.PopAt(i)
		require.Zero(t, v)
		require.False(t, ok)
	}
	require.Equal(t, []int{0, 10, 20, 30}, b.Elements())
	require.Empty(t, h.items)
	v, ok := b.PopAt(2)
	require.True(t, ok)
	require.Equal(t, 20, v)
	require.Equal(t, []int{0, 10, 30}, b.Elements())
	v, ok = b.PeekAt(2)
	require.True(t, ok)
	require.Equal(t, 30, v)
	v, ok = b.PopAt(2)
	require.True(t, ok)
	require.Equal(t, 30, v)
	v, ok = b.PopAt(0)
	require.True(t, ok)
	require.Zero(t, v)
	v, ok = b.Pop()
	require.True(t, ok)
	require.Equal(t, 10, v)
	v, ok = b.Pop()
	require.False(t, ok)
	require.Zero(t, v)
	require.Equal(t, []any{20, 30, 0, 10}, h.items)
	require.True(t, b.CanPush())
	t.Log("PopAt(2) removed 20, preserved [0 10 30]; zero-valued head returned (0, true); " +
		"invalid reads left contents and hooks unchanged")
}

func TestBufferNilIsPresent(t *testing.T) {
	b := NewBuffer[*int]("nil", 1)
	b.Push(nil)
	v, ok := b.Peek()
	require.True(t, ok)
	require.Nil(t, v)
	v, ok = b.Pop()
	require.True(t, ok)
	require.Nil(t, v)
	v, ok = b.Pop()
	require.False(t, ok)
	require.Nil(t, v)
}

func TestBufferUpdateFrontPresence(t *testing.T) {
	b := NewBuffer[int]("head", 1)
	require.False(t, b.UpdateFront(9))
	require.Zero(t, b.Size())
	b.Push(5)
	require.True(t, b.UpdateFront(0))
	v, ok := b.Peek()
	require.True(t, ok)
	require.Zero(t, v)
}

func TestBufferIndexedJSONRoundTrip(t *testing.T) {
	b := NewBuffer[int]("bank", 4)
	for _, v := range []int{0, 1, 2} {
		b.Push(v)
	}
	b.PopAt(1)
	data, err := json.Marshal(b)
	require.NoError(t, err)
	var restored Buffer[int]
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, b.Name(), restored.Name())
	require.Equal(t, b.Capacity(), restored.Capacity())
	require.Equal(t, []int{0, 2}, restored.Elements())
	v, ok := restored.Pop()
	require.True(t, ok)
	require.Zero(t, v)
}
