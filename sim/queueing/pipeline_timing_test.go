package queueing

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelineLatencyEqualsNumStages(t *testing.T) {
	for _, stages := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("stages=%d", stages), func(t *testing.T) {
			p := MakePipeline[int](1, stages)
			sink := newPostBuf(1)
			p.Accept(42)
			for tick := 1; tick < stages; tick++ {
				require.True(t, p.Tick(sink), "tick %d must make progress", tick)
				require.Zero(t, sink.Size(), "tick %d must not output early", tick)
			}
			require.True(t, p.Tick(sink))
			require.Equal(t, []int{42}, sink.Elements())
			require.Empty(t, p.Stages())
		})
	}
}

func TestPipelineAcceptsEveryCycle(t *testing.T) {
	for _, stages := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("stages=%d", stages), func(t *testing.T) {
			p := MakePipeline[int](1, stages)
			sink := newPostBuf(100)
			const n = 10
			for i := 0; i < n; i++ {
				require.True(t, p.CanAccept(), "item %d must be accepted", i)
				p.Accept(i)
				p.Tick(sink)
			}
			for len(p.Stages()) > 0 {
				p.Tick(sink)
			}
			require.Len(t, sink.Elements(), n)
		})
	}
}
