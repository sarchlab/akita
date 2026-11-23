package queueing

import (
	"fmt"
	"slices"
)

// fastPipelineEntry is one resident item. Progress counts the cycles the item
// has been in the pipeline, capped at numStages. Its stage is derived: an item
// with k items ahead in its lane is at stage min(Progress, numStages-1-k).
// Storing progress rather than the stage distinguishes an item blocked at a
// stage from one that just arrived there, so releasing a lane cannot move an
// item twice in one cycle.
type fastPipelineEntry[T any] struct {
	Lane     int `json:"lane"`
	Progress int `json:"progress"`
	Item     T   `json:"item"`
}

// FastPipeline is a generic multi-lane, multi-stage pipeline that the consumer
// pulls from. It has no engine dependency and does no per-cycle work: AdvanceTo
// jumps over any number of elapsed cycles at a cost proportional to the
// resident items.
//
// Each lane is an independent FIFO holding at most numStages items. An item
// accepted in cycle c is ready in cycle c+numStages. A ready item stays in the
// final stage, holding its lane's occupancy, until the consumer pops it.
//
// Drive it in this order in each observed cycle:
//
//  1. AdvanceTo(cycle).
//  2. Pull with ReadyLane, PeekLane, UpdateLane, and PopLane. Leave an item in
//     place to apply backpressure.
//  3. Admit with CanAccept and Accept.
//  4. Ask NextReadyCycle and NextAcceptCycle when to come back.
//
// A pop releases its lane in the same cycle: items behind it close the gap by
// one stage, so stage 0 of a full lane can accept in that cycle. Each lane
// delivers at most one item and accepts at most one item per cycle.
//
// FastPipeline is not synchronized. Embed it by value in component state and
// use it through a pointer; copies share storage.
type FastPipeline[T any] struct {
	width     int
	numStages int
	// cycle is the last cycle all entries were synchronized to.
	cycle uint64
	// entries holds resident items, oldest accepted first.
	entries []fastPipelineEntry[T]
	// laneScratch is per-call scratch sized to width. It is not simulation
	// state and is excluded from checkpoints.
	laneScratch []int
}

// MakeFastPipeline creates a pipeline with the given width (lanes) and number
// of stages. numStages must be positive. A pipeline with no lanes never accepts.
func MakeFastPipeline[T any](width, numStages int) FastPipeline[T] {
	if numStages < 1 {
		panic("fast pipeline must have at least one stage")
	}

	return FastPipeline[T]{width: width, numStages: numStages}
}

// Cycle returns the cycle the pipeline was last synchronized to.
func (p *FastPipeline[T]) Cycle() uint64 {
	return p.cycle
}

// Size returns the number of resident items.
func (p *FastPipeline[T]) Size() int {
	return len(p.entries)
}

// AdvanceTo brings every item forward to cycle. It panics if cycle is earlier
// than the last synchronized cycle.
func (p *FastPipeline[T]) AdvanceTo(cycle uint64) {
	if cycle < p.cycle {
		panic(fmt.Sprintf("fast pipeline cannot move from cycle %d back to %d", p.cycle, cycle))
	}

	elapsed := cycle - p.cycle
	p.cycle = cycle
	if elapsed == 0 {
		return
	}

	for i := range p.entries {
		e := &p.entries[i]
		if remaining := p.numStages - e.Progress; elapsed >= uint64(remaining) {
			e.Progress = p.numStages
		} else {
			e.Progress += int(elapsed)
		}
	}
}

// SkipTo moves the pipeline's clock to cycle without advancing any item, so the
// skipped cycles do not count. Use it to resume after a freeze. It panics if
// cycle is earlier than the last synchronized cycle.
func (p *FastPipeline[T]) SkipTo(cycle uint64) {
	if cycle < p.cycle {
		panic(fmt.Sprintf("fast pipeline cannot move from cycle %d back to %d", p.cycle, cycle))
	}

	p.cycle = cycle
}

// CanAccept returns true if some lane has stage 0 free in the current cycle.
func (p *FastPipeline[T]) CanAccept() bool {
	lane, _ := p.admission()

	return lane >= 0
}

// Accept inserts an item at stage 0 of the lowest free lane. It panics when no
// lane can accept; callers must check CanAccept first.
func (p *FastPipeline[T]) Accept(item T) {
	lane, _ := p.admission()
	if lane < 0 {
		panic("fast pipeline has no free input lane")
	}

	p.entries = append(p.entries, fastPipelineEntry[T]{Lane: lane, Item: item})
}

// admission returns the lowest lane with stage 0 free now, or -1, and whether
// a lane that is busy only with this cycle's accept frees up next cycle.
func (p *FastPipeline[T]) admission() (lane int, nextCycle bool) {
	if p.width <= 0 {
		return -1, false
	}
	if len(p.entries) == 0 {
		return 0, false
	}

	counts := p.scratch()
	for i := range p.entries {
		counts[p.entries[i].Lane]++
	}
	// An item with no progress holds stage 0 for this cycle only, unless its
	// lane is full.
	for i := range p.entries {
		e := &p.entries[i]
		if e.Progress == 0 && counts[e.Lane] < p.numStages {
			counts[e.Lane] = p.numStages
			nextCycle = true
		}
	}

	for l, count := range counts {
		if count < p.numStages {
			return l, nextCycle
		}
	}

	return -1, nextCycle
}

// ReadyLane returns the lane of the oldest accepted item that is ready. Only
// the head of a lane can be ready, and a blocked lane does not hold back others.
func (p *FastPipeline[T]) ReadyLane() (int, bool) {
	if len(p.entries) == 0 {
		return 0, false
	}

	seen := p.scratch()
	for i := range p.entries {
		e := &p.entries[i]
		if seen[e.Lane] != 0 {
			continue
		}
		seen[e.Lane] = 1
		if e.Progress == p.numStages {
			return e.Lane, true
		}
	}

	return 0, false
}

// PeekLane returns the ready item at the head of lane without removing it.
func (p *FastPipeline[T]) PeekLane(lane int) (T, bool) {
	i, ok := p.readyIndex(lane)
	if !ok {
		var zero T
		return zero, false
	}

	return p.entries[i].Item, true
}

// UpdateLane replaces the ready item at the head of lane and reports whether
// one existed. Use it to keep completed work attached while backpressured.
func (p *FastPipeline[T]) UpdateLane(lane int, item T) bool {
	i, ok := p.readyIndex(lane)
	if !ok {
		return false
	}
	p.entries[i].Item = item

	return true
}

// PopLane removes and returns the ready item at the head of lane. The next item
// in the lane cannot become ready before the next cycle.
func (p *FastPipeline[T]) PopLane(lane int) (T, bool) {
	i, ok := p.readyIndex(lane)
	if !ok {
		var zero T
		return zero, false
	}

	item := p.entries[i].Item
	p.entries = slices.Delete(p.entries, i, i+1)
	for j := i; j < len(p.entries); j++ {
		if e := &p.entries[j]; e.Lane == lane {
			e.Progress = min(e.Progress, p.numStages-1)
			break
		}
	}

	return item, true
}

// readyIndex returns the entry index of lane's head if that head is ready.
func (p *FastPipeline[T]) readyIndex(lane int) (int, bool) {
	for i := range p.entries {
		if p.entries[i].Lane == lane {
			return i, p.entries[i].Progress == p.numStages
		}
	}

	return 0, false
}

// NextReadyCycle returns the earliest cycle at which a lane head is ready,
// assuming no further pops. It returns the current cycle if one is ready now,
// and false when the pipeline is empty.
func (p *FastPipeline[T]) NextReadyCycle() (uint64, bool) {
	if len(p.entries) == 0 {
		return 0, false
	}

	seen := p.scratch()
	best := 0
	found := false
	for i := range p.entries {
		e := &p.entries[i]
		if seen[e.Lane] != 0 {
			continue
		}
		seen[e.Lane] = 1
		if wait := p.numStages - e.Progress; !found || wait < best {
			best, found = wait, true
		}
	}

	return p.cycle + uint64(best), true
}

// NextAcceptCycle returns the earliest cycle at which CanAccept becomes true
// without any further pop: the current cycle or the next one. It returns false
// when every lane is full, because only a pop can then release space; wait for
// NextReadyCycle and pop instead.
func (p *FastPipeline[T]) NextAcceptCycle() (uint64, bool) {
	lane, nextCycle := p.admission()
	switch {
	case lane >= 0:
		return p.cycle, true
	case nextCycle:
		return p.cycle + 1, true
	default:
		return 0, false
	}
}

// Stages returns a copy of the resident items, oldest accepted first, each with
// its current lane and stage. It is intended for inspection and testing.
func (p *FastPipeline[T]) Stages() []PipelineStage[T] {
	out := make([]PipelineStage[T], len(p.entries))
	if len(p.entries) == 0 {
		return out
	}

	ahead := p.scratch()
	for i := range p.entries {
		e := &p.entries[i]
		out[i] = PipelineStage[T]{
			Lane:  e.Lane,
			Stage: min(e.Progress, p.numStages-1-ahead[e.Lane]),
			Item:  e.Item,
		}
		ahead[e.Lane]++
	}

	return out
}

// Clear removes all items. The pipeline stays at its current cycle.
func (p *FastPipeline[T]) Clear() {
	p.entries = nil
}

// scratch returns zeroed per-lane scratch.
func (p *FastPipeline[T]) scratch() []int {
	if len(p.laneScratch) != p.width {
		p.laneScratch = make([]int, p.width)
	} else {
		clear(p.laneScratch)
	}

	return p.laneScratch
}
