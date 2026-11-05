package queueing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func fullPipeline(width, stages int) Pipeline[int] {
	p := MakePipeline[int](width, stages)
	for stage := 0; stage < stages; stage++ {
		for lane := 0; lane < width; lane++ {
			p.stages = append(p.stages, PipelineStage[int]{
				Lane: lane, Stage: stage, Item: stage*width + lane,
			})
		}
	}
	return p
}

func TestPipelineReusesOccupancyWhileBlocked(t *testing.T) {
	p := fullPipeline(8, 64)
	sink := nopSink[int]{}
	before := p.Stages()
	require.False(t, p.Tick(sink))
	allocs := testing.AllocsPerRun(100, func() { p.Tick(sink) })
	require.Zero(t, allocs)
	require.Equal(t, before, p.Stages())
}

func TestPipelineRestoreRebuildsOccupancy(t *testing.T) {
	p := fullPipeline(2, 4)
	blocked := nopSink[int]{}
	p.Tick(blocked) // Warm the scratch buffer before taking a checkpoint.
	data, err := json.Marshal(p)
	require.NoError(t, err)

	// Restore over a used pipeline with different geometry and scratch contents.
	restored := fullPipeline(8, 64)
	restored.Tick(blocked)
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Nil(t, restored.occupancy)

	sink := MakeBuffer[int](1)
	restoredSink := MakeBuffer[int](1)
	for tick := 0; tick < 32; tick++ {
		require.Equal(t, p.Tick(&sink), restored.Tick(&restoredSink))
		require.Equal(t, p.Stages(), restored.Stages())
		require.Equal(t, sink.Elements(), restoredSink.Elements())
		if tick%2 == 1 { // Alternate backpressure and output availability.
			sink.Pop()
			restoredSink.Pop()
		}
	}
	require.Empty(t, p.Stages())

	// Clear and refill reuse must not inherit occupied slots from the old stream.
	p.Clear()
	p.Accept(99)
	require.Equal(t, []int{99}, drainPipeline(&p))
}

func BenchmarkPipelineBlocked(b *testing.B) {
	p := fullPipeline(8, 64)
	sink := nopSink[int]{}
	p.Tick(sink)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Tick(sink)
	}
}
