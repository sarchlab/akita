package tlb

import (
	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCalculatedOutputQueueStallsAndDrains(t *testing.T) {
	for _, width := range []int{1, 4} {
		c := &Comp{}
		c.Spec = DefaultSpec()
		c.Spec.Bottom.Destinations = []messaging.RemotePort{"Walker.Top"}
		c.Spec.NumReqPerCycle = width
		c.Spec.Latency = 1
		c.State.Initialize(c)
		require.Equal(t, width, c.State.BufferItems.Capacity())
		clock := testsim.NewClock()
		c.State.Pipeline.BindClock(clock)
		for range width {
			c.State.Pipeline.Push(pipelineTLBReqState{})
		}
		step := func() {
			clock.Step(c.Spec.Freq)
			c.State.Pipeline.DrainReady(&c.State.BufferItems)
		}
		step()
		for range width {
			c.State.Pipeline.Push(pipelineTLBReqState{})
		}
		for range 5 {
			step()
		}
		require.Equal(t, width, c.State.BufferItems.Size())
		require.Len(t, c.State.Pipeline.Stages(), width)
		for range 2 * width {
			_, ok := c.State.BufferItems.Pop()
			require.True(t, ok)
			step()
		}
		require.Zero(t, c.State.BufferItems.Size())
		require.Empty(t, c.State.Pipeline.Stages())
		t.Logf("width %d: full output queue stalls lookup pipeline; all %d entries drain", width, 2*width)
	}
	spec := DefaultSpec()
	spec.NumReqPerCycle = 0
	require.Error(t, validateSpec(spec))
}
