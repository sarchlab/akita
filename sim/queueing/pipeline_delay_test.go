package queueing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineDelayedTraversal(t *testing.T) {
	for _, stages := range []int{1, 2, 4} {
		for _, delay := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("stages=%d/delay=%d", stages, delay), func(t *testing.T) {
				p := MakePipeline[int](1, stages)
				sink := newPostBuf(1)
				p.AcceptWithDelay(42, delay)
				for tick := 1; tick < stages+delay; tick++ {
					require.True(t, p.Tick(sink), "tick %d must make progress", tick)
					require.Zero(t, sink.Size(), "tick %d must not output early", tick)
				}
				require.True(t, p.Tick(sink))
				require.Equal(t, []int{42}, sink.Elements())
				require.Empty(t, p.Stages())
				require.True(t, p.CanAccept())
			})
		}
	}
}

func TestPipelineSingleStageDelayedBackpressure(t *testing.T) {
	p := MakePipeline[int](1, 1)
	sink := newPostBuf(1)
	sink.Push(99)
	p.AcceptWithDelay(42, 1)
	require.True(t, p.Tick(sink), "waiting must progress even with a full sink")
	require.Equal(t, 0, p.Stages()[0].CycleLeft)
	require.False(t, p.Tick(sink), "ready item must wait for sink capacity")
	require.False(t, p.CanAccept())
	value, ok := sink.Pop()
	require.True(t, ok)
	require.Equal(t, 99, value)
	require.True(t, p.Tick(sink))
	require.Equal(t, []int{42}, sink.Elements())
	require.Empty(t, p.Stages())
}

func TestPipelineSingleStageMixedDelays(t *testing.T) {
	p := MakePipeline[int](3, 1)
	sink := newPostBuf(3)
	p.AcceptWithDelay(10, 2)
	p.AcceptWithDelay(20, 0)
	p.AcceptWithDelay(30, 1)
	require.True(t, p.Tick(sink))
	require.Equal(t, []int{20}, sink.Elements())
	require.Len(t, p.Stages(), 2)
	for _, stage := range p.Stages() {
		if stage.Item == 10 {
			require.Equal(t, 1, stage.CycleLeft, "compaction must not decrement twice")
		} else {
			require.Equal(t, 30, stage.Item)
			require.Zero(t, stage.CycleLeft)
		}
	}
	require.True(t, p.Tick(sink))
	require.Equal(t, []int{20, 30}, sink.Elements())
	require.True(t, p.Tick(sink))
	require.Equal(t, []int{20, 30, 10}, sink.Elements())
	require.Empty(t, p.Stages())
}
