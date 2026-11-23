package queueing

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func restoredCopy(t *testing.T, p Pipeline[int]) Pipeline[int] {
	t.Helper()
	data, err := json.Marshal(p)
	require.NoError(t, err)
	var restored Pipeline[int]
	require.NoError(t, json.Unmarshal(data, &restored))
	return restored
}

func TestPipelineShallowCopySeesSharedAdvance(t *testing.T) {
	for name, writerIsCopy := range map[string]bool{"copy ticks": true, "original ticks": false} {
		t.Run(name, func(t *testing.T) {
			first := MakePipeline[int](1, 2)
			first.Accept(1)
			second := first

			writer, stale := &first, &second
			if writerIsCopy {
				writer, stale = &second, &first
			}
			require.True(t, writer.Tick(nopSink[int]{}))

			// The item left stage 0 in the shared array for both values.
			want := []PipelineStage[int]{{Lane: 0, Stage: 1, Item: 1}}
			require.Equal(t, want, stale.Stages())
			require.True(t, stale.CanAccept())

			restored := restoredCopy(t, *stale)
			require.True(t, restored.CanAccept())

			stale.Accept(2)
			require.Equal(t, PipelineStage[int]{Lane: 0, Stage: 0, Item: 2}, stale.Stages()[1])
			require.False(t, stale.CanAccept())
			require.Panics(t, func() { stale.Accept(3) })

			// Reallocation on append left the writer's view unchanged.
			require.Equal(t, want, writer.Stages())
			require.True(t, writer.CanAccept())
		})
	}
}

func TestPipelineShallowCopiesOverwriteSharedTail(t *testing.T) {
	p := MakePipeline[int](2, 2)
	p.Accept(1)
	p.Accept(2)
	sink := MakeBuffer[int](1)
	p.Tick(&sink) // Both items reach the last stage.
	p.Tick(&sink) // One leaves, keeping spare capacity in the array.
	require.Equal(t, []PipelineStage[int]{{Lane: 0, Stage: 1, Item: 1}}, p.Stages())

	q := p
	p.Accept(3)
	q.Accept(4) // Overwrites item 3 in the slot both values share.
	require.Equal(t, 4, p.Stages()[1].Item)

	p.Accept(5)
	require.False(t, p.CanAccept())
	require.True(t, q.CanAccept())
	restored := restoredCopy(t, p)
	require.False(t, restored.CanAccept())
}

func TestPipelineStaleCopyStallsOnAliasedSlot(t *testing.T) {
	p := MakePipeline[int](1, 3)
	p.restoreStages([]PipelineStage[int]{
		{Lane: 0, Stage: 2, Item: 1},
		{Lane: 0, Stage: 0, Item: 2},
		{Lane: 0, Stage: 1, Item: 3},
	})
	q := p

	sink := MakeBuffer[int](2)
	require.True(t, q.Tick(&sink))
	require.Equal(t, []PipelineStage[int]{
		{Lane: 0, Stage: 2, Item: 3},
		{Lane: 0, Stage: 1, Item: 2},
		{Lane: 0, Stage: 1, Item: 3}, // Stale tail, visible only through p.
	}, p.Stages())

	// Only one of the two items sharing stage 1 can advance.
	require.True(t, p.Tick(&sink))
	require.Equal(t, []PipelineStage[int]{
		{Lane: 0, Stage: 2, Item: 3},
		{Lane: 0, Stage: 1, Item: 2},
	}, p.Stages())
}

func TestPipelineRestoredAliasedSlotsKeepScanning(t *testing.T) {
	var p Pipeline[int]
	require.NoError(t, json.Unmarshal([]byte(`{"width":2,"num_stages":3,"stages":[
		{"lane":0,"stage":0,"item":1},
		{"lane":0,"stage":0,"item":2}
	]}`), &p))

	// Two items fill the count while lane 1 is still free.
	require.False(t, p.CanAccept())
	p.Accept(3)
	require.Equal(t, PipelineStage[int]{Lane: 1, Stage: 0, Item: 3}, p.Stages()[2])

	require.True(t, p.Tick(nopSink[int]{}))
	require.Equal(t, []PipelineStage[int]{
		{Lane: 0, Stage: 1, Item: 1},
		{Lane: 0, Stage: 0, Item: 2},
		{Lane: 1, Stage: 1, Item: 3},
	}, p.Stages())
}

func TestPipelineClearAndRestoreDetachFromCopies(t *testing.T) {
	p := MakePipeline[int](1, 2)
	p.Accept(1)

	cleared := p
	cleared.Clear()
	require.True(t, p.tracked(), "Clear does not write the shared stages")

	replaced := p
	require.NoError(t, json.Unmarshal(
		[]byte(`{"width":1,"num_stages":2,"stages":[]}`), &replaced))
	require.True(t, p.tracked(), "restore does not write the shared stages")

	// Neither detached value observes later writes to the old stages.
	p.Tick(nopSink[int]{})
	require.True(t, cleared.CanAccept())
	require.Empty(t, cleared.Stages())
	require.True(t, replaced.CanAccept())
	cleared.Accept(2)
	replaced.Accept(3)
	require.True(t, cleared.tracked())
	require.True(t, replaced.tracked())
	require.Equal(t, []PipelineStage[int]{{Lane: 0, Stage: 1, Item: 1}}, p.Stages())
}

func TestPipelineLiveValueStaysTracked(t *testing.T) {
	p := MakePipeline[int](2, 3)
	sink := MakeBuffer[int](1)
	for i := 0; i < 16; i++ {
		if p.CanAccept() {
			p.Accept(i)
		}
		p.Tick(&sink)
		require.True(t, p.tracked())
		require.Equal(t, p.countStageZero(), p.stageZero)
		if i%3 == 0 {
			sink.Pop()
		}
	}

	// A blocked tick writes nothing, so a copy keeps its cache.
	full := fullPipeline(2, 2)
	reader := full
	require.False(t, reader.Tick(nopSink[int]{}))
	require.True(t, full.tracked())
	require.False(t, full.CanAccept())
}

func TestPipelineEmptyReaderCopyDoesNotRaceOwner(t *testing.T) {
	p := MakePipeline[int](1, 2)
	sink := MakeBuffer[int](1)
	p.Accept(1)
	p.Tick(&sink)
	p.Tick(&sink)
	sink.Pop()
	require.Empty(t, p.Stages())

	// An empty view reads no stage slots, so it may be polled during writes.
	reader := p
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			assert.True(t, reader.CanAccept())
		}
	}()
	for i := 0; i < 1000; i++ {
		p.Accept(i)
		p.Tick(&sink)
		p.Tick(&sink)
		sink.Pop()
	}
	<-done
}
