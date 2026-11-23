package queueing

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// testPipelineFreq has a one-picosecond period, so cycles and times coincide.
const testPipelineFreq = 1000 * timing.GHz

// pipelineClock is a clock a test moves by cycle.
type pipelineClock struct {
	now timing.VTimeInPicoSec
}

func (c *pipelineClock) CurrentTime() timing.VTimeInPicoSec { return c.now }

func (c *pipelineClock) to(cycle uint64) { c.now = timing.VTimeInPicoSec(cycle) }

func newTestPipeline(width, stages int) (*FastPipeline[int], *pipelineClock) {
	clock := &pipelineClock{}
	p := MakeFastPipeline[int](width, stages, testPipelineFreq, clock)

	return &p, clock
}

func stagesOf(p *FastPipeline[int]) map[int][2]int {
	out := map[int][2]int{}
	for _, s := range p.Stages() {
		out[s.Item] = [2]int{s.Lane, s.Stage}
	}

	return out
}

func TestFastPipelineLatencyEqualsNumStages(t *testing.T) {
	for _, stages := range []int{1, 2, 4, 9} {
		t.Run(fmt.Sprintf("stages=%d", stages), func(t *testing.T) {
			p, clock := newTestPipeline(1, stages)
			clock.to(100)
			p.Push(42)

			due, ok := p.NextReadyTime()
			require.True(t, ok)
			require.Equal(t, timing.VTimeInPicoSec(100+stages), due)

			clock.to(uint64(due) - 1)
			_, ready := p.Peek()
			require.False(t, ready, "must not be ready a cycle early")

			clock.to(uint64(due))
			item, ready := p.Peek()
			require.True(t, ready)
			require.Equal(t, 42, item)
			item, ok = p.Pop()
			require.True(t, ok)
			require.Equal(t, 42, item)
			require.Zero(t, p.Size())
		})
	}
}

func TestFastPipelineReadsTimeThroughFrequency(t *testing.T) {
	for _, freq := range []timing.Freq{1 * timing.GHz, 3 * timing.GHz, 7 * timing.MHz} {
		clock := &pipelineClock{}
		p := MakeFastPipeline[int](1, 4, freq, clock)
		clock.now = 10 * freq.Period()
		p.Push(1)

		due, ok := p.NextReadyTime()
		require.True(t, ok)
		require.Equal(t, 14*freq.Period(), due, "deadlines are clock edges of %d", freq)

		clock.now = due - 1
		_, ready := p.Peek()
		require.False(t, ready)
		clock.now = due
		_, ready = p.Peek()
		require.True(t, ready)
	}
}

func TestFastPipelineSameEdgeSeesOneCycle(t *testing.T) {
	p, clock := newTestPipeline(1, 2)
	clock.to(5)
	p.Push(1)
	clock.to(7)

	// Peek, Pop, and Push on one edge agree: the pop frees the lane for the
	// push, and the pushed item takes its full latency from this edge.
	item, ok := p.Peek()
	require.True(t, ok)
	require.Equal(t, 1, item)
	item, ok = p.Pop()
	require.True(t, ok)
	require.Equal(t, 1, item)
	require.True(t, p.CanPush())
	p.Push(2)
	_, ok = p.Peek()
	require.False(t, ok)

	due, ok := p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(9), due)
}

func TestFastPipelinePeekPicksOldestReadyHead(t *testing.T) {
	p, clock := newTestPipeline(3, 2)
	p.Push(10) // lane 0
	p.Push(20) // lane 1
	clock.to(1)
	p.Push(11) // behind 10
	p.Push(21) // behind 20
	p.Push(30) // lane 2
	clock.to(2)

	for _, want := range []int{10, 20} {
		item, ok := p.Peek()
		require.True(t, ok)
		require.Equal(t, want, item)
		popped, ok := p.Pop()
		require.True(t, ok)
		require.Equal(t, want, popped)
	}

	_, ok := p.Peek()
	require.False(t, ok, "followers and lane 2 are not ready in this cycle")
	clock.to(3)
	for _, want := range []int{11, 21, 30} {
		item, ok := p.Pop()
		require.True(t, ok)
		require.Equal(t, want, item)
	}
}

func TestFastPipelineObservationDoesNotMoveItems(t *testing.T) {
	p, clock := newTestPipeline(2, 4)
	p.Push(1)
	clock.to(2)
	p.Push(2)
	before, err := json.Marshal(p)
	require.NoError(t, err)

	clock.to(3)
	require.Equal(t, map[int][2]int{1: {0, 3}, 2: {0, 1}}, stagesOf(p))
	require.True(t, p.CanPush())
	_, _ = p.Peek()
	_, _ = p.NextReadyTime()
	_, _ = p.NextPushTime()

	after, err := json.Marshal(p)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestFastPipelineSustainsOneItemPerLanePerCycle(t *testing.T) {
	p, clock := newTestPipeline(1, 4)
	popped := []int{}

	for cycle := uint64(0); cycle < 20; cycle++ {
		clock.to(cycle)
		if item, ok := p.Pop(); ok {
			popped = append(popped, item)
		}
		require.True(t, p.CanPush(), "cycle %d", cycle)
		p.Push(int(cycle))
	}

	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, popped)
}

func TestFastPipelineLargeGapCompactsEachLane(t *testing.T) {
	p, clock := newTestPipeline(2, 5)
	p.Push(0)
	p.Push(1)
	clock.to(1)
	p.Push(2)
	clock.to(2)
	p.Push(3)

	clock.to(math.MaxUint64 / 2)

	require.Equal(t, map[int][2]int{
		0: {0, 4},
		1: {1, 4},
		2: {0, 3},
		3: {0, 2},
	}, stagesOf(p))
}

func TestFastPipelineCatchUpDoesNotAllocateOrScaleWithGap(t *testing.T) {
	p, clock := newTestPipeline(4, 64)
	for range 4 {
		p.Push(1)
	}

	cycle := uint64(0)
	allocs := testing.AllocsPerRun(100, func() {
		cycle += 1 << 40
		clock.to(cycle)
		p.sync()
	})
	require.Zero(t, allocs)
}

func TestFastPipelineLanesBackpressureIndependently(t *testing.T) {
	p, clock := newTestPipeline(2, 2)
	p.Push(10) // lane 0
	clock.to(1)
	p.Push(11) // lane 0
	p.Push(20) // lane 1
	clock.to(3)

	sink := MakeBuffer[int](1)
	require.True(t, p.DrainReady(&sink))
	require.Equal(t, []int{10}, sink.Elements())

	// With its head gone, lane 0's follower waits a cycle, and lane 1 is ready.
	item, ok := p.Peek()
	require.True(t, ok)
	require.Equal(t, 20, item)

	clock.to(4)
	for _, want := range []int{11, 20} {
		item, ok = p.Pop()
		require.True(t, ok)
		require.Equal(t, want, item)
	}
}

func TestFastPipelineUpdateStaysAttachedWhileBlocked(t *testing.T) {
	p, clock := newTestPipeline(1, 1)
	p.Push(1)
	p.UpdateReady(func(int) int {
		require.Fail(t, "an item cannot be updated before it is ready")
		return 0
	})

	clock.to(1)
	p.UpdateReady(func(int) int { return 7 })
	clock.to(1000)

	item, ok := p.Peek()
	require.True(t, ok)
	require.Equal(t, 7, item)
	require.Equal(t, 1, p.Size())
}

func TestFastPipelineUpdateReadyVisitsEachReadyHeadOldestFirst(t *testing.T) {
	p, clock := newTestPipeline(3, 2)
	p.Push(10) // lane 0
	p.Push(20) // lane 1
	clock.to(1)
	p.Push(11) // behind 10 in lane 0
	p.Push(21) // behind 20 in lane 1
	p.Push(30) // lane 2, not ready until cycle 3
	clock.to(2)

	visited := []int{}
	p.UpdateReady(func(item int) int {
		visited = append(visited, item)
		return item + 1
	})
	require.Equal(t, []int{10, 20}, visited)

	item, ok := p.Pop()
	require.True(t, ok)
	require.Equal(t, 11, item)
	item, ok = p.Pop()
	require.True(t, ok)
	require.Equal(t, 21, item)
	_, ok = p.Pop()
	require.False(t, ok)
}

func TestFastPipelineAdmission(t *testing.T) {
	t.Run("one item per lane per cycle", func(t *testing.T) {
		p, clock := newTestPipeline(2, 3)
		p.Push(0)
		p.Push(1)
		require.False(t, p.CanPush())

		due, ok := p.NextPushTime()
		require.True(t, ok)
		require.Equal(t, timing.VTimeInPicoSec(1), due)

		clock.to(1)
		require.True(t, p.CanPush())
		due, ok = p.NextPushTime()
		require.True(t, ok)
		require.Equal(t, timing.VTimeInPicoSec(1), due)
	})

	t.Run("full pipeline needs a pop", func(t *testing.T) {
		p, clock := newTestPipeline(1, 2)
		p.Push(0)
		clock.to(1)
		p.Push(1)
		clock.to(100)

		require.False(t, p.CanPush())
		_, ok := p.NextPushTime()
		require.False(t, ok)
		require.Panics(t, func() { p.Push(2) })
	})

	t.Run("a pop admits in the same cycle at stage zero", func(t *testing.T) {
		p, clock := newTestPipeline(1, 3)
		p.Push(0)
		clock.to(1)
		p.Push(1)
		clock.to(2)
		p.Push(2)
		clock.to(50)
		require.False(t, p.CanPush())

		_, ok := p.Pop()
		require.True(t, ok)
		require.True(t, p.CanPush())
		p.Push(3)

		require.Equal(t, map[int][2]int{
			1: {0, 2},
			2: {0, 1},
			3: {0, 0},
		}, stagesOf(p))
	})

	t.Run("a pop does not double-step an item that just moved", func(t *testing.T) {
		p, clock := newTestPipeline(1, 4)
		p.Push(0)
		clock.to(2)
		p.Push(1) // two stages behind the head
		clock.to(4)

		require.Equal(t, map[int][2]int{0: {0, 3}, 1: {0, 2}}, stagesOf(p))
		_, ok := p.Pop()
		require.True(t, ok)
		require.Equal(t, map[int][2]int{1: {0, 2}}, stagesOf(p))

		due, ok := p.NextReadyTime()
		require.True(t, ok)
		require.Equal(t, timing.VTimeInPicoSec(6), due)
	})

	t.Run("no lanes never accepts", func(t *testing.T) {
		p, _ := newTestPipeline(0, 3)
		require.False(t, p.CanPush())
		_, ok := p.NextPushTime()
		require.False(t, ok)
	})
}

func TestFastPipelineNextReadyTime(t *testing.T) {
	p, clock := newTestPipeline(2, 4)
	_, ok := p.NextReadyTime()
	require.False(t, ok)

	clock.to(10)
	p.Push(0) // lane 0
	p.Push(1) // lane 1
	clock.to(12)
	p.Push(2) // behind item 0 in lane 0

	due, ok := p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(14), due)

	clock.to(20)
	_, ok = p.NextReadyTime()
	require.False(t, ok, "heads that are already ready are not future work")

	_, ok = p.Pop()
	require.True(t, ok)
	due, ok = p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(21), due, "a follower leaves the cycle after its predecessor")
}

func TestFastPipelineNextReadyTimeSeesPastABlockedHead(t *testing.T) {
	p, clock := newTestPipeline(2, 4)
	p.Push(0) // lane 0
	clock.to(1)
	p.Push(1) // behind item 0
	p.Push(2) // lane 1
	clock.to(4)

	item, ready := p.Peek()
	require.True(t, ready)
	require.Equal(t, 0, item)
	due, ok := p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(5), due, "lane 1's head is still on its way")
}

func TestFastPipelinePauseFreezesItems(t *testing.T) {
	p, clock := newTestPipeline(1, 4)
	p.Push(0)
	clock.to(3)

	// The edge that pauses does not count: the item stops at cycle 2.
	p.Pause()
	require.Equal(t, map[int][2]int{0: {0, 2}}, stagesOf(p))
	_, ok := p.NextReadyTime()
	require.False(t, ok, "no deadline passes while paused")
	_, ok = p.NextPushTime()
	require.False(t, ok)

	clock.to(1000)
	p.Pause()
	require.Equal(t, map[int][2]int{0: {0, 2}}, stagesOf(p))

	// The edge that resumes counts, so the item is at progress 3 and has one
	// cycle to go.
	p.Resume()
	require.Equal(t, map[int][2]int{0: {0, 3}}, stagesOf(p))
	due, ok := p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(1001), due)
}

func TestFastPipelinePauseAfterSameEdgeWorkKeepsThatEdge(t *testing.T) {
	p, clock := newTestPipeline(1, 4)
	p.Push(0)
	clock.to(2)
	require.False(t, p.DrainReady(&Buffer[int]{}))

	// The pipeline has already handled this edge, so pausing keeps its position.
	p.Pause()
	require.Equal(t, map[int][2]int{0: {0, 2}}, stagesOf(p))

	clock.to(10)
	p.Resume()
	require.Equal(t, map[int][2]int{0: {0, 3}}, stagesOf(p))
}

func TestFastPipelineClearKeepsPause(t *testing.T) {
	p, clock := newTestPipeline(1, 2)
	p.Push(0)
	clock.to(5)
	p.Pause()
	p.Clear()
	require.Zero(t, p.Size())

	clock.to(9)
	p.Resume()
	p.Push(1)
	due, ok := p.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(11), due)
}

func TestFastPipelineRejectsInvalidUse(t *testing.T) {
	clock := &pipelineClock{}
	require.Panics(t, func() { MakeFastPipeline[int](1, 0, testPipelineFreq, clock) })
	require.Panics(t, func() { MakeFastPipeline[int](1, 1, 0, clock) })
	require.Panics(t, func() { MakeFastPipeline[int](1, 1, testPipelineFreq, nil) })

	p, moved := newTestPipeline(1, 1)
	moved.to(5)
	p.Push(1)
	moved.to(4)
	require.Panics(t, func() { p.Push(2) }, "the clock must not move backward")
	require.Panics(t, func() { p.BindClock(nil) })
}

func TestFastPipelineJSONRoundTrip(t *testing.T) {
	p, clock := newTestPipeline(2, 4)
	clock.to(7)
	p.Push(1)
	clock.to(8)
	p.Push(2)
	p.Push(3)

	data, err := json.Marshal(p)
	require.NoError(t, err)
	require.NotContains(t, string(data), "clock")

	var got FastPipeline[int]
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, p.Stages(), got.Stages(), "an unbound copy reports its saved positions")
	require.Panics(t, func() { got.CanPush() }, "an unbound copy cannot read time")
	got.BindClock(clock)
	require.False(t, got.CanPush())

	// The restored pipeline must then behave identically.
	for cycle := uint64(9); cycle < 20; cycle++ {
		clock.to(cycle)
		require.Equal(t, p.Stages(), got.Stages())
		want, ok := p.Pop()
		item, gotOK := got.Pop()
		require.Equal(t, ok, gotOK)
		require.Equal(t, want, item)
	}
}

func TestFastPipelineJSONCatchesUpAfterASleep(t *testing.T) {
	p, clock := newTestPipeline(1, 10)
	clock.to(3)
	p.Push(1)
	clock.to(8) // no call synchronizes the pipeline at this edge

	data, err := json.Marshal(p)
	require.NoError(t, err)
	var got FastPipeline[int]
	require.NoError(t, json.Unmarshal(data, &got))
	got.BindClock(clock)

	require.Equal(t, p.Stages(), got.Stages())
	want, _ := p.NextReadyTime()
	due, ok := got.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, want, due)
	require.Equal(t, timing.VTimeInPicoSec(13), due)
}

func TestFastPipelineJSONKeepsPause(t *testing.T) {
	p, clock := newTestPipeline(1, 4)
	p.Push(1)
	clock.to(3)
	p.Pause()

	data, err := json.Marshal(p)
	require.NoError(t, err)
	var got FastPipeline[int]
	require.NoError(t, json.Unmarshal(data, &got))
	got.BindClock(clock)

	clock.to(50)
	require.Equal(t, map[int][2]int{1: {0, 2}}, stagesOf(&got))
	got.Resume()
	due, ok := got.NextReadyTime()
	require.True(t, ok)
	require.Equal(t, timing.VTimeInPicoSec(51), due)
}

func TestFastPipelineJSONRestoreIntoLiveValueKeepsClock(t *testing.T) {
	p, clock := newTestPipeline(1, 2)
	p.Push(1)
	data, err := json.Marshal(p)
	require.NoError(t, err)

	target, targetClock := newTestPipeline(3, 7)
	require.NoError(t, json.Unmarshal(data, target))
	targetClock.to(2)
	item, ok := target.Pop()
	require.True(t, ok)
	require.Equal(t, 1, item)
	require.Zero(t, clock.now)
}

func TestFastPipelineJSONRejectsEntriesOutsideGeometry(t *testing.T) {
	for name, data := range map[string]string{
		"lane":     `{"width":1,"num_stages":2,"freq":1000,"entries":[{"lane":1,"progress":0,"item":1}]}`,
		"progress": `{"width":1,"num_stages":2,"freq":1000,"entries":[{"lane":0,"progress":3,"item":1}]}`,
		"occupancy": `{"width":1,"num_stages":1,"freq":1000,"entries":[` +
			`{"lane":0,"progress":1,"item":1},{"lane":0,"progress":0,"item":2}]}`,
		"frequency": `{"width":1,"num_stages":2,"entries":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var p FastPipeline[int]
			require.Error(t, json.Unmarshal([]byte(data), &p))
		})
	}
}

func TestFastPipelineJSONRejectsPushPipelineSnapshot(t *testing.T) {
	live := MakePipeline[int](1, 3)
	live.Accept(7)
	empty := MakePipeline[int](1, 3)

	for name, snapshot := range map[string]Pipeline[int]{"live": live, "empty": empty} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(snapshot)
			require.NoError(t, err)

			p, _ := newTestPipeline(2, 5)
			p.Push(1)
			require.ErrorContains(t, json.Unmarshal(data, p), "push Pipeline snapshot")
			require.Equal(t, 1, p.Size(), "a rejected snapshot must leave the pipeline unchanged")
		})
	}
}

func TestFastPipelineDrainReadyIsOldestFirstAndKeepsBlockedHeads(t *testing.T) {
	p, clock := newTestPipeline(3, 2)
	p.Push(1)
	p.Push(2)
	p.Push(3)
	clock.to(2)

	sink := MakeBuffer[int](2)
	require.True(t, p.DrainReady(&sink))
	require.Equal(t, []int{1, 2}, sink.Elements())
	require.Equal(t, 1, p.Size(), "a head that does not fit stays in its lane")
	require.False(t, p.DrainReady(&sink), "a full sink takes nothing")

	sink.Pop()
	require.True(t, p.DrainReady(&sink))
	require.Equal(t, []int{2, 3}, sink.Elements())
	require.Zero(t, p.Size())
	require.False(t, p.DrainReady(&sink), "an empty pipeline has nothing to drain")
}

func TestFastPipelineDrainReadyDeliversOnePerLanePerCycle(t *testing.T) {
	p, clock := newTestPipeline(1, 2)
	p.Push(1)
	clock.to(1)
	p.Push(2)
	clock.to(5)

	sink := MakeBuffer[int](4)
	require.True(t, p.DrainReady(&sink))
	require.Equal(t, []int{1}, sink.Elements())
	require.False(t, p.DrainReady(&sink), "the follower is not ready until the next cycle")

	clock.to(6)
	require.True(t, p.DrainReady(&sink))
	require.Equal(t, []int{1, 2}, sink.Elements())
}

// TestFastPipelineDrainReadyConservesItemsUnderBackpressure checks that random
// backpressure neither loses nor duplicates an item, shortens its latency, or
// reorders a lane.
func TestFastPipelineDrainReadyConservesItemsUnderBackpressure(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		width, numStages := 1+rng.Intn(4), 1+rng.Intn(6)
		capacity := 1 + rng.Intn(width+1)
		p, clock := newTestPipeline(width, numStages)
		sink := MakeBuffer[int](capacity)
		acceptedAt, laneOf, lastInLane := map[int]uint64{}, map[int]int{}, map[int]int{}
		delivered := map[int]bool{}
		next := 0

		pop := func(cycle uint64) {
			item, ok := sink.Pop()
			if !ok {
				return
			}
			require.False(t, delivered[item], "seed %d item %d delivered twice", seed, item)
			delivered[item] = true
			require.GreaterOrEqual(t, cycle, acceptedAt[item]+uint64(numStages), "seed %d", seed)
			if last, seen := lastInLane[laneOf[item]]; seen {
				require.Greater(t, item, last, "seed %d lane reordered", seed)
			}
			lastInLane[laneOf[item]] = item
		}

		cycle := uint64(1)
		for ; cycle < 400; cycle++ {
			clock.to(cycle)
			p.DrainReady(&sink)
			for range rng.Intn(capacity + 1) {
				pop(cycle)
			}
			for rng.Intn(3) != 0 && p.CanPush() {
				p.Push(next)
				stages := p.Stages()
				laneOf[next], acceptedAt[next] = stages[len(stages)-1].Lane, cycle
				next++
			}
			require.LessOrEqual(t, p.Size(), width*numStages, "seed %d", seed)
		}

		for ; p.Size() > 0 || sink.Size() > 0; cycle++ {
			clock.to(cycle)
			p.DrainReady(&sink)
			pop(cycle)
		}
		require.Len(t, delivered, next, "seed %d", seed)
	}
}

// TestFastPipelineDrainMatchesPushPipelineWithoutBackpressure checks that items
// leave in the same cycles as the deprecated pipeline when the sink never fills.
func TestFastPipelineDrainMatchesPushPipelineWithoutBackpressure(t *testing.T) {
	for seed := int64(0); seed < 50; seed++ {
		rng := rand.New(rand.NewSource(seed))
		width, numStages := 1+rng.Intn(4), 1+rng.Intn(6)
		push, pushBuf := MakePipeline[int](width, numStages), MakeBuffer[int](width)
		fast, clock := newTestPipeline(width, numStages)
		fastBuf := MakeBuffer[int](width)
		next := 0

		for cycle := uint64(1); cycle < 300; cycle++ {
			push.Tick(&pushBuf)
			clock.to(cycle)
			fast.DrainReady(&fastBuf)
			require.ElementsMatch(t, pushBuf.Elements(), fastBuf.Elements(), "seed %d cycle %d", seed, cycle)
			pushBuf.Clear()
			fastBuf.Clear()

			require.Equal(t, push.CanAccept(), fast.CanPush(), "seed %d cycle %d", seed, cycle)
			for rng.Intn(3) != 0 && fast.CanPush() {
				push.Accept(next)
				fast.Push(next)
				next++
			}
		}
	}
}

// steppedPipeline is a cycle-by-cycle reference: lanes[l][s] holds the item at
// stage s of lane l, or -1.
type steppedPipeline struct {
	lanes [][]int
}

func newSteppedPipeline(width, stages int) *steppedPipeline {
	s := &steppedPipeline{lanes: make([][]int, width)}
	for l := range s.lanes {
		s.lanes[l] = make([]int, stages)
		for i := range s.lanes[l] {
			s.lanes[l][i] = -1
		}
	}

	return s
}

// readyHeads returns the items in the final stages, oldest pushed first. Items
// are numbered in push order.
func (s *steppedPipeline) readyHeads() []int {
	heads := []int{}
	for _, lane := range s.lanes {
		if item := lane[len(lane)-1]; item >= 0 {
			heads = append(heads, item)
		}
	}
	slices.Sort(heads)

	return heads
}

// tick runs one cycle: the items in leaving exit, the remaining items shift
// toward the output from high stage to low, then new items enter.
func (s *steppedPipeline) tick(leaving, accept []int) {
	for _, lane := range s.lanes {
		last := len(lane) - 1
		if slices.Contains(leaving, lane[last]) {
			lane[last] = -1
		}
		for i := last - 1; i >= 0; i-- {
			if lane[i] >= 0 && lane[i+1] < 0 {
				lane[i+1], lane[i] = lane[i], -1
			}
		}
	}
	for _, item := range accept {
		for _, lane := range s.lanes {
			if lane[0] < 0 {
				lane[0] = item
				break
			}
		}
	}
}

// TestFastPipelineMatchesSteppedReference compares random traffic and a random
// number of oldest-first pops against a model that moves every item every
// cycle, including jumps over idle cycles.
func TestFastPipelineMatchesSteppedReference(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		width := 1 + rng.Intn(4)
		stages := 1 + rng.Intn(7)
		p, clock := newTestPipeline(width, stages)
		ref := newSteppedPipeline(width, stages)
		next := 0
		cycle := uint64(0)

		for range 400 {
			// Jump over a run of cycles in which nothing leaves or enters.
			gap := 1 + rng.Intn(4)*rng.Intn(3)
			for range gap - 1 {
				ref.tick(nil, nil)
			}
			cycle += uint64(gap)
			clock.to(cycle)

			heads := ref.readyHeads()
			leaving := heads[:rng.Intn(len(heads)+1)]
			got := []int{}
			for range leaving {
				item, ok := p.Pop()
				require.True(t, ok, "seed %d cycle %d", seed, cycle)
				got = append(got, item)
			}
			require.Equal(t, leaving, got, "seed %d cycle %d", seed, cycle)

			accept := []int{}
			for rng.Intn(3) != 0 && p.CanPush() {
				p.Push(next)
				accept = append(accept, next)
				next++
			}

			ref.tick(leaving, accept)

			wantStages := map[int][2]int{}
			for l, lane := range ref.lanes {
				for s, item := range lane {
					if item >= 0 {
						wantStages[item] = [2]int{l, s}
					}
				}
			}
			require.Equal(t, wantStages, stagesOf(p), "seed %d cycle %d", seed, cycle)
		}
	}
}

// BenchmarkFastPipelineSaturatedCycle measures one cycle of a full-rate
// pipeline: every lane delivers one item and accepts one item.
func BenchmarkFastPipelineSaturatedCycle(b *testing.B) {
	p, clock := newTestPipeline(4, 8)
	cycle := uint64(0)

	b.ReportAllocs()
	for b.Loop() {
		cycle++
		clock.to(cycle)
		for {
			if _, ok := p.Peek(); !ok {
				break
			}
			p.Pop()
		}
		for p.CanPush() {
			p.Push(1)
		}
		_, _ = p.NextReadyTime()
	}
}

// TestWideFastPipelineIsNotLimitedByAOneSlotBuffer pins an intentional
// difference: a one-entry buffer caps a wide push Pipeline at one completion
// per cycle, while each FastPipeline lane completes independently.
func TestWideFastPipelineIsNotLimitedByAOneSlotBuffer(t *testing.T) {
	const width, numStages, cycles = 2, 4, 1000

	push := MakePipeline[int](width, numStages)
	buf := MakeBuffer[int](1)
	pushed := 0
	for range cycles {
		if _, ok := buf.Pop(); ok {
			pushed++
		}
		push.Tick(&buf)
		for push.CanAccept() {
			push.Accept(0)
		}
	}

	fast, clock := newTestPipeline(width, numStages+1)
	pulled := 0
	for cycle := range uint64(cycles) {
		clock.to(cycle)
		for {
			if _, ok := fast.Pop(); !ok {
				break
			}
			pulled++
		}
		for fast.CanPush() {
			fast.Push(0)
		}
	}

	t.Logf("completions in %d cycles: push+buffer %d, fast %d", cycles, pushed, pulled)
	require.InDelta(t, cycles, pushed, numStages+2)
	require.InDelta(t, width*cycles, pulled, width*(numStages+2))
}

// TestFastPipelineMatchesPipelineWithOneSlotBuffer shows a one-lane FastPipeline
// of N+1 stages times exactly like a push Pipeline of N stages draining into a
// one-entry buffer, under random admission and backpressure.
func TestFastPipelineMatchesPipelineWithOneSlotBuffer(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		numStages := 1 + rng.Intn(8)
		fast, clock := newTestPipeline(1, numStages+1)
		push := MakePipeline[int](1, numStages)
		buf := MakeBuffer[int](1)
		next := 0

		for cycle := uint64(1); cycle < 600; cycle++ {
			clock.to(cycle)

			if rng.Intn(3) != 0 {
				want, wantOK := buf.Pop()
				got, gotOK := fast.Pop()
				require.Equal(t, wantOK, gotOK, "seed %d cycle %d", seed, cycle)
				require.Equal(t, want, got, "seed %d cycle %d", seed, cycle)
			}
			push.Tick(&buf)

			require.Equal(t, push.CanAccept(), fast.CanPush(), "seed %d cycle %d", seed, cycle)
			if rng.Intn(4) != 0 && fast.CanPush() {
				fast.Push(next)
				push.Accept(next)
				next++
			}
		}
	}
}
