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

// callbackSink accepts every item, running onCanPush before each admission and
// onPush after each insertion.
type callbackSink struct {
	calls     int
	onCanPush func(call int)
	onPush    func(call int)
	pushed    []int
}

func (s *callbackSink) CanPush() bool {
	s.calls++
	if s.onCanPush != nil {
		s.onCanPush(s.calls)
	}
	return true
}

func (s *callbackSink) Push(item int) {
	s.pushed = append(s.pushed, item)
	if s.onPush != nil {
		s.onPush(len(s.pushed))
	}
}

func TestPipelineSinkPanicAfterSwapInvalidatesAdmission(t *testing.T) {
	p := MakePipeline[int](2, 2)
	p.Accept(1)
	p.Accept(2)
	p.Tick(nopSink[int]{})
	p.Accept(3)
	q := p

	// The first output swaps item 3 over item 2 and leaves the slice unshortened.
	want := []PipelineStage[int]{
		{Lane: 0, Stage: 1, Item: 1},
		{Lane: 0, Stage: 0, Item: 3},
		{Lane: 0, Stage: 0, Item: 3},
	}
	sink := &callbackSink{onCanPush: func(call int) {
		if call == 2 {
			require.Equal(t, want, p.Stages())
			require.False(t, p.CanAccept(), "a later sink call sees the swap")
			panic("sink failed")
		}
	}}
	require.PanicsWithValue(t, "sink failed", func() { p.Tick(sink) })
	require.Equal(t, []int{2}, sink.pushed)

	for name, view := range map[string]*Pipeline[int]{"receiver": &p, "copy": &q} {
		require.Equal(t, want, view.Stages(), name)
		require.False(t, view.CanAccept(), name)
		restored := restoredCopy(t, *view)
		require.False(t, restored.CanAccept(), name)
	}

	// The duplicate occupies lane 0 twice, leaving lane 1 open as it always did.
	p.Accept(4)
	require.Equal(t, PipelineStage[int]{Lane: 1, Stage: 0, Item: 4}, p.Stages()[3])
}

func TestPipelineSinkReplacementIsInvalidatedBeforeLaterSwap(t *testing.T) {
	for name, tc := range map[string]struct {
		replace func(p *Pipeline[int])
		want    []PipelineStage[int]
	}{
		"fresh storage": {
			replace: func(p *Pipeline[int]) {
				fresh := MakePipeline[int](3, 2)
				fresh.restoreStages([]PipelineStage[int]{
					{Lane: 0, Stage: 1, Item: 10},
					{Lane: 1, Stage: 1, Item: 11},
					{Lane: 0, Stage: 0, Item: 12},
					{Lane: 1, Stage: 0, Item: 13},
				})
				*p = fresh
			},
			want: []PipelineStage[int]{
				{Lane: 0, Stage: 1, Item: 10},
				{Lane: 0, Stage: 0, Item: 12},
				{Lane: 0, Stage: 0, Item: 12},
				{Lane: 1, Stage: 0, Item: 13},
			},
		},
		"up-to-date alias": {
			replace: func(p *Pipeline[int]) {
				p.seenGen = p.stageGen.Load()
				p.stageZero = p.countStageZero()
			},
			want: []PipelineStage[int]{
				{Lane: 0, Stage: 1, Item: 1},
				{Lane: 0, Stage: 0, Item: 4},
				{Lane: 0, Stage: 0, Item: 4},
				{Lane: 0, Stage: 0, Item: 4},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := MakePipeline[int](3, 2)
			p.restoreStages([]PipelineStage[int]{
				{Lane: 0, Stage: 1, Item: 1},
				{Lane: 1, Stage: 1, Item: 2},
				{Lane: 2, Stage: 1, Item: 3},
				{Lane: 0, Stage: 0, Item: 4},
			})

			sink := &callbackSink{
				onPush: func(pushed int) {
					if pushed == 2 {
						tc.replace(&p)
					}
				},
				onCanPush: func(call int) {
					if call == 3 {
						require.Equal(t, tc.want, p.Stages())
						require.False(t, p.CanAccept(), "third callback")
						panic("sink failed")
					}
				},
			}
			require.PanicsWithValue(t, "sink failed", func() { p.Tick(sink) })

			require.Equal(t, tc.want, p.Stages())
			require.False(t, p.CanAccept(), "after recovery")
			restored := restoredCopy(t, p)
			require.False(t, restored.CanAccept(), "after checkpoint")
		})
	}
}

func TestPipelineSinkPanicBeforeWriteKeepsCache(t *testing.T) {
	p := fullPipeline(2, 2)
	q := p
	sink := &callbackSink{onCanPush: func(int) { panic("sink failed") }}
	require.Panics(t, func() { p.Tick(sink) })

	require.True(t, p.tracked())
	require.True(t, q.tracked())
	require.False(t, p.CanAccept())
}

func TestPipelineCopyWriteDuringTickIsNotAdopted(t *testing.T) {
	p := MakePipeline[int](2, 2)
	p.Accept(1)
	p.Accept(2)
	p.Tick(nopSink[int]{})
	q := p

	// The copy writes before the receiver's first swap publishes its own write.
	sink := &callbackSink{onCanPush: func(call int) {
		if call == 1 {
			q.Accept(3)
		}
	}}
	require.True(t, p.Tick(sink))
	require.False(t, p.tracked())
	require.True(t, p.CanAccept())
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
