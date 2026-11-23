package queueing

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			p := MakeFastPipeline[int](1, stages)
			p.AdvanceTo(100)
			p.Accept(42)

			due, ok := p.NextReadyCycle()
			require.True(t, ok)
			require.Equal(t, uint64(100+stages), due)

			p.AdvanceTo(due - 1)
			_, ready := p.ReadyLane()
			require.False(t, ready, "must not be ready a cycle early")

			p.AdvanceTo(due)
			lane, ready := p.ReadyLane()
			require.True(t, ready)
			item, ok := p.PopLane(lane)
			require.True(t, ok)
			require.Equal(t, 42, item)
			require.Zero(t, p.Size())
		})
	}
}

func TestFastPipelineSustainsOneItemPerLanePerCycle(t *testing.T) {
	p := MakeFastPipeline[int](1, 4)
	popped := []int{}

	for cycle := uint64(0); cycle < 20; cycle++ {
		p.AdvanceTo(cycle)
		if lane, ok := p.ReadyLane(); ok {
			item, _ := p.PopLane(lane)
			popped = append(popped, item)
		}
		require.True(t, p.CanAccept(), "cycle %d", cycle)
		p.Accept(int(cycle))
	}

	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, popped)
}

func TestFastPipelineLargeGapCompactsEachLane(t *testing.T) {
	p := MakeFastPipeline[int](2, 5)
	p.Accept(0)
	p.Accept(1)
	p.AdvanceTo(1)
	p.Accept(2)
	p.AdvanceTo(2)
	p.Accept(3)

	p.AdvanceTo(math.MaxUint64 / 2)

	require.Equal(t, map[int][2]int{
		0: {0, 4},
		1: {1, 4},
		2: {0, 3},
		3: {0, 2},
	}, stagesOf(&p))
	require.Equal(t, uint64(math.MaxUint64/2), p.Cycle())
}

func TestFastPipelineAdvanceDoesNotAllocateOrScaleWithGap(t *testing.T) {
	p := MakeFastPipeline[int](4, 64)
	for range 4 {
		p.Accept(1)
	}

	cycle := uint64(0)
	allocs := testing.AllocsPerRun(100, func() {
		cycle += 1 << 40
		p.AdvanceTo(cycle)
	})
	require.Zero(t, allocs)
}

func TestFastPipelineLanesBackpressureIndependently(t *testing.T) {
	p := MakeFastPipeline[int](2, 2)
	p.Accept(10) // lane 0
	p.Accept(20) // lane 1
	p.AdvanceTo(1)
	p.Accept(11) // lane 0
	p.Accept(21) // lane 1
	p.AdvanceTo(2)

	// Oldest first across lanes, but lane 0 is left blocked.
	lane, ok := p.ReadyLane()
	require.True(t, ok)
	require.Equal(t, 0, lane)

	item, ok := p.PopLane(1)
	require.True(t, ok)
	require.Equal(t, 20, item)

	_, ok = p.PeekLane(1)
	require.False(t, ok, "a follower must not leave in its predecessor's cycle")

	p.AdvanceTo(3)
	item, ok = p.PopLane(1)
	require.True(t, ok)
	require.Equal(t, 21, item)

	// Lane 0 never moved, and its FIFO order is intact.
	item, ok = p.PopLane(0)
	require.True(t, ok)
	require.Equal(t, 10, item)
	p.AdvanceTo(4)
	item, ok = p.PopLane(0)
	require.True(t, ok)
	require.Equal(t, 11, item)
}

func TestFastPipelineUpdateStaysAttachedWhileBlocked(t *testing.T) {
	p := MakeFastPipeline[int](1, 1)
	p.Accept(1)
	require.False(t, p.UpdateLane(0, 5), "an item cannot be updated before it is ready")

	p.AdvanceTo(1)
	require.True(t, p.UpdateLane(0, 7))
	p.AdvanceTo(1000)

	item, ok := p.PeekLane(0)
	require.True(t, ok)
	require.Equal(t, 7, item)
	require.Equal(t, 1, p.Size())
}

func TestFastPipelineAdmission(t *testing.T) {
	t.Run("one item per lane per cycle", func(t *testing.T) {
		p := MakeFastPipeline[int](2, 3)
		p.Accept(0)
		p.Accept(1)
		require.False(t, p.CanAccept())

		due, ok := p.NextAcceptCycle()
		require.True(t, ok)
		require.Equal(t, uint64(1), due)

		p.AdvanceTo(1)
		require.True(t, p.CanAccept())
		due, ok = p.NextAcceptCycle()
		require.True(t, ok)
		require.Equal(t, uint64(1), due)
	})

	t.Run("full pipeline needs a pop", func(t *testing.T) {
		p := MakeFastPipeline[int](1, 2)
		p.Accept(0)
		p.AdvanceTo(1)
		p.Accept(1)
		p.AdvanceTo(100)

		require.False(t, p.CanAccept())
		_, ok := p.NextAcceptCycle()
		require.False(t, ok)
		require.Panics(t, func() { p.Accept(2) })
	})

	t.Run("a pop admits in the same cycle at stage zero", func(t *testing.T) {
		p := MakeFastPipeline[int](1, 3)
		p.Accept(0)
		p.AdvanceTo(1)
		p.Accept(1)
		p.AdvanceTo(2)
		p.Accept(2)
		p.AdvanceTo(50)
		require.False(t, p.CanAccept())

		_, ok := p.PopLane(0)
		require.True(t, ok)
		require.True(t, p.CanAccept())
		p.Accept(3)

		require.Equal(t, map[int][2]int{
			1: {0, 2},
			2: {0, 1},
			3: {0, 0},
		}, stagesOf(&p))
	})

	t.Run("a pop does not double-step an item that just moved", func(t *testing.T) {
		p := MakeFastPipeline[int](1, 4)
		p.Accept(0)
		p.AdvanceTo(2)
		p.Accept(1) // two stages behind the head
		p.AdvanceTo(4)

		require.Equal(t, map[int][2]int{0: {0, 3}, 1: {0, 2}}, stagesOf(&p))
		_, ok := p.PopLane(0)
		require.True(t, ok)
		require.Equal(t, map[int][2]int{1: {0, 2}}, stagesOf(&p))

		due, ok := p.NextReadyCycle()
		require.True(t, ok)
		require.Equal(t, uint64(6), due)
	})

	t.Run("no lanes never accepts", func(t *testing.T) {
		p := MakeFastPipeline[int](0, 3)
		require.False(t, p.CanAccept())
		_, ok := p.NextAcceptCycle()
		require.False(t, ok)
	})
}

func TestFastPipelineNextReadyCycle(t *testing.T) {
	p := MakeFastPipeline[int](2, 4)
	_, ok := p.NextReadyCycle()
	require.False(t, ok)

	p.AdvanceTo(10)
	p.Accept(0) // lane 0
	p.Accept(1) // lane 1
	p.AdvanceTo(12)
	p.Accept(2) // behind item 0 in lane 0

	due, ok := p.NextReadyCycle()
	require.True(t, ok)
	require.Equal(t, uint64(14), due)

	p.AdvanceTo(20)
	due, ok = p.NextReadyCycle()
	require.True(t, ok)
	require.Equal(t, uint64(20), due, "a blocked item is due now")

	_, ok = p.PopLane(0)
	require.True(t, ok)
	due, ok = p.NextReadyCycle()
	require.True(t, ok)
	require.Equal(t, uint64(20), due, "the other lane is still ready")

	_, ok = p.PopLane(1)
	require.True(t, ok)
	due, ok = p.NextReadyCycle()
	require.True(t, ok)
	require.Equal(t, uint64(21), due, "a follower leaves the cycle after its predecessor")
}

func TestFastPipelineSkipToFreezesItems(t *testing.T) {
	p := MakeFastPipeline[int](1, 4)
	p.Accept(0)
	p.AdvanceTo(2)

	p.SkipTo(1000)
	require.Equal(t, map[int][2]int{0: {0, 2}}, stagesOf(&p))

	due, ok := p.NextReadyCycle()
	require.True(t, ok)
	require.Equal(t, uint64(1002), due)
}

func TestFastPipelineRejectsInvalidUse(t *testing.T) {
	require.Panics(t, func() { MakeFastPipeline[int](1, 0) })

	p := MakeFastPipeline[int](1, 1)
	p.AdvanceTo(5)
	require.Panics(t, func() { p.AdvanceTo(4) })
	require.Panics(t, func() { p.SkipTo(4) })

	_, ok := p.PopLane(7)
	require.False(t, ok)
}

func TestFastPipelineJSONRoundTrip(t *testing.T) {
	p := MakeFastPipeline[int](2, 4)
	p.AdvanceTo(7)
	p.Accept(1)
	p.AdvanceTo(8)
	p.Accept(2)
	p.Accept(3)

	data, err := json.Marshal(p)
	require.NoError(t, err)

	var got FastPipeline[int]
	require.NoError(t, json.Unmarshal(data, &got))
	require.Equal(t, p.Stages(), got.Stages())
	require.Equal(t, p.Cycle(), got.Cycle())
	require.False(t, got.CanAccept())

	// The restored pipeline must then behave identically.
	for cycle := uint64(9); cycle < 20; cycle++ {
		p.AdvanceTo(cycle)
		got.AdvanceTo(cycle)
		require.Equal(t, p.Stages(), got.Stages())
		lane, ok := p.ReadyLane()
		gotLane, gotOK := got.ReadyLane()
		require.Equal(t, ok, gotOK)
		require.Equal(t, lane, gotLane)
		if ok {
			want, _ := p.PopLane(lane)
			item, _ := got.PopLane(lane)
			require.Equal(t, want, item)
		}
	}
}

func TestFastPipelineJSONRejectsEntriesOutsideGeometry(t *testing.T) {
	for name, data := range map[string]string{
		"lane":     `{"width":1,"num_stages":2,"entries":[{"lane":1,"progress":0,"item":1}]}`,
		"progress": `{"width":1,"num_stages":2,"entries":[{"lane":0,"progress":3,"item":1}]}`,
		"occupancy": `{"width":1,"num_stages":1,"entries":[` +
			`{"lane":0,"progress":1,"item":1},{"lane":0,"progress":0,"item":2}]}`,
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

			p := MakeFastPipeline[int](2, 5)
			p.Accept(1)
			require.ErrorContains(t, json.Unmarshal(data, &p), "push Pipeline snapshot")
			require.Equal(t, 1, p.Size(), "a rejected snapshot must leave the pipeline unchanged")
		})
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

// tick runs one cycle: pop picks which ready lane heads leave, the remaining
// items shift toward the output from high stage to low, then new items enter.
func (s *steppedPipeline) tick(pop func(lane int) bool, accept []int) []int {
	out := []int{}
	for l, lane := range s.lanes {
		last := len(lane) - 1
		if lane[last] >= 0 && pop(l) {
			out = append(out, lane[last])
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

	return out
}

// TestFastPipelineMatchesSteppedReference compares random traffic and random
// per-lane backpressure against a model that moves every item every cycle,
// including jumps over idle cycles.
func TestFastPipelineMatchesSteppedReference(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		width := 1 + rng.Intn(4)
		stages := 1 + rng.Intn(7)
		p := MakeFastPipeline[int](width, stages)
		ref := newSteppedPipeline(width, stages)
		next := 0
		cycle := uint64(0)

		for range 400 {
			// Jump over a run of cycles in which nothing leaves or enters.
			gap := 1 + rng.Intn(4)*rng.Intn(3)
			for range gap - 1 {
				ref.tick(func(int) bool { return false }, nil)
			}
			cycle += uint64(gap)
			p.AdvanceTo(cycle)

			blocked := make([]bool, width)
			for l := range blocked {
				blocked[l] = rng.Intn(3) == 0
			}

			got := []int{}
			for l := range width {
				if blocked[l] {
					continue
				}
				if item, ok := p.PopLane(l); ok {
					got = append(got, item)
				}
			}

			accept := []int{}
			for rng.Intn(3) != 0 && p.CanAccept() {
				p.Accept(next)
				accept = append(accept, next)
				next++
			}

			want := ref.tick(func(l int) bool { return !blocked[l] }, accept)
			require.ElementsMatch(t, want, got, "seed %d cycle %d", seed, cycle)

			wantStages := map[int][2]int{}
			for l, lane := range ref.lanes {
				for s, item := range lane {
					if item >= 0 {
						wantStages[item] = [2]int{l, s}
					}
				}
			}
			require.Equal(t, wantStages, stagesOf(&p), "seed %d cycle %d", seed, cycle)
		}
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

	fast := MakeFastPipeline[int](width, numStages+1)
	pulled := 0
	for cycle := range uint64(cycles) {
		fast.AdvanceTo(cycle)
		for {
			lane, ok := fast.ReadyLane()
			if !ok {
				break
			}
			fast.PopLane(lane)
			pulled++
		}
		for fast.CanAccept() {
			fast.Accept(0)
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
		fast := MakeFastPipeline[int](1, numStages+1)
		push := MakePipeline[int](1, numStages)
		buf := MakeBuffer[int](1)
		next := 0

		for cycle := uint64(1); cycle < 600; cycle++ {
			fast.AdvanceTo(cycle)

			if rng.Intn(3) != 0 {
				want, wantOK := buf.Pop()
				got, gotOK := fast.PopLane(0)
				require.Equal(t, wantOK, gotOK, "seed %d cycle %d", seed, cycle)
				require.Equal(t, want, got, "seed %d cycle %d", seed, cycle)
			}
			push.Tick(&buf)

			require.Equal(t, push.CanAccept(), fast.CanAccept(), "seed %d cycle %d", seed, cycle)
			if rng.Intn(4) != 0 && fast.CanAccept() {
				fast.Accept(next)
				push.Accept(next)
				next++
			}
		}
	}
}
