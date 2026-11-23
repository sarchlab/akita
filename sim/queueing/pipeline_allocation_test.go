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

func TestPipelineRestoreFullIngress(t *testing.T) {
	data, err := json.Marshal(fullPipeline(2, 2))
	require.NoError(t, err)

	restored := MakePipeline[int](2, 2)
	require.NoError(t, json.Unmarshal(data, &restored))
	require.False(t, restored.CanAccept())
	require.Panics(t, func() { restored.Accept(8) })

	// Releasing lane 1 frees only that lane at stage 0.
	sink := MakeBuffer[int](1)
	require.True(t, restored.Tick(&sink))
	require.Equal(t, []int{3}, sink.Elements())
	require.True(t, restored.CanAccept())
	restored.Accept(8)
	require.Equal(t, []PipelineStage[int]{
		{Lane: 0, Stage: 0, Item: 0},
		{Lane: 1, Stage: 1, Item: 1},
		{Lane: 0, Stage: 1, Item: 2},
		{Lane: 1, Stage: 0, Item: 8},
	}, restored.Stages())
	require.False(t, restored.CanAccept())

	restored.Clear()
	require.True(t, restored.CanAccept())
	restored.Accept(9)
	require.Equal(t, []PipelineStage[int]{{Lane: 0, Stage: 0, Item: 9}}, restored.Stages())
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
