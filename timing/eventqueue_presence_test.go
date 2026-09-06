package timing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventQueuePresence(t *testing.T) {
	for _, safe := range []bool{false, true} {
		var q EventQueue = newUnsafeEventQueue()
		if safe {
			q = NewEventQueue()
		}
		for cycle := 0; cycle < 2; cycle++ {
			evt, ok := q.Peek()
			require.False(t, ok)
			require.Nil(t, evt)
			evt, ok = q.Pop()
			require.False(t, ok)
			require.Nil(t, evt)
			first := &benchEvent{t: 0}
			second := &benchEvent{t: 1}
			q.Push(second)
			q.Push(first)
			evt, ok = q.Peek()
			require.True(t, ok)
			require.Same(t, first, evt)
			require.Equal(t, 2, q.Len())
			evt, ok = q.Pop()
			require.True(t, ok)
			require.Same(t, first, evt)
			evt, ok = q.Pop()
			require.True(t, ok)
			require.Same(t, second, evt)
		}
	}
	t.Log("Both event queues returned (nil, false) when empty and remained usable for ordered push/peek/pop")
}
