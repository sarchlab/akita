package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestOutputCapacityIndependentOfLookupWidth(t *testing.T) {
	for _, capacity := range []int{1, 8} {
		c := &Comp{}
		c.Spec = DefaultSpec()
		c.Spec.Bottom.Destinations = []messaging.RemotePort{"Walker.Top"}
		c.Spec.NumReqPerCycle = 3
		c.Spec.OutputBufferCapacity = capacity
		c.Spec.Latency = 1
		c.State.Initialize(c)
		for range 3 {
			require.True(t, c.State.Pipeline.CanAccept())
			c.State.Pipeline.Accept(pipelineTLBReqState{})
		}
		require.False(t, c.State.Pipeline.CanAccept())
		c.State.Pipeline.Tick(&c.State.BufferItems)
		require.Equal(t, min(3, capacity), c.State.BufferItems.Size())
		count := 0
		for c.State.BufferItems.Size() > 0 || len(c.State.Pipeline.Stages()) > 0 {
			for c.State.BufferItems.Size() > 0 {
				c.State.BufferItems.Pop()
				count++
			}
			c.State.Pipeline.Tick(&c.State.BufferItems)
		}
		require.Equal(t, 3, count)
	}
	spec := DefaultSpec()
	spec.OutputBufferCapacity = 0
	require.Error(t, validateSpec(spec))
}
