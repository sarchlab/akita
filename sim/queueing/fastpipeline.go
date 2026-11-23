package queueing

import (
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/sim/timing"
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
// pulls from. It reads time from an injected clock and does no per-cycle work:
// items catch up lazily, at a cost proportional to the resident items however
// many cycles have passed. It never schedules events; its owner asks
// NextReadyTime when to come back and calls TickAt.
//
// Each lane is an independent FIFO holding at most numStages items. An item
// pushed in cycle c is ready in cycle c+numStages. A ready item stays in the
// final stage, holding its lane's occupancy, until the consumer pops it. Lanes
// are private: Push takes the lowest free lane, and Peek and Pop use the oldest
// pushed ready head.
//
// Drive it in this order in each observed cycle:
//
//  1. Apply completion work to every ready lane head with UpdateReady.
//  2. Pull with Peek and, once the output has accepted the item, Pop. Leave an
//     item in place to apply backpressure.
//  3. Admit with CanPush and Push.
//  4. Ask NextReadyTime and NextPushTime when to come back.
//
// A pop releases its lane in the same cycle: items behind it close the gap by
// one stage, so stage 0 of a full lane can accept in that cycle. Each lane
// delivers at most one item and accepts at most one item per cycle.
//
// FastPipeline is not synchronized. Embed it by value in component state and
// use it through a pointer; copies share storage. The clock is not serialized:
// a decoded pipeline must get one from BindClock before it is used.
type FastPipeline[T any] struct {
	width     int
	numStages int
	freq      timing.Freq
	// clock is the read-only time source, bound at runtime.
	clock timing.TimeTeller
	// cycle is the last cycle all entries were synchronized to.
	cycle uint64
	// paused stops item progress at cycle until Resume.
	paused bool
	// entries holds resident items, oldest pushed first.
	entries []fastPipelineEntry[T]
	// laneScratch is per-call scratch sized to width. It is not simulation
	// state and is excluded from checkpoints.
	laneScratch []int
}

// MakeFastPipeline creates a pipeline with the given width (lanes) and number
// of stages. freq converts the clock's time to cycles and is ordinarily the
// owner's frequency; clock is ordinarily the owner. numStages and freq must be
// positive and clock non-nil. A pipeline with no lanes never accepts.
func MakeFastPipeline[T any](
	width, numStages int, freq timing.Freq, clock timing.TimeTeller,
) FastPipeline[T] {
	if numStages < 1 {
		panic("fast pipeline must have at least one stage")
	}
	if freq == 0 {
		panic("fast pipeline must have a nonzero frequency")
	}
	if clock == nil {
		panic("fast pipeline must have a clock")
	}

	return FastPipeline[T]{width: width, numStages: numStages, freq: freq, clock: clock}
}

// BindClock sets the time source. Checkpoints do not save it, so the owner
// binds it again after loading.
func (p *FastPipeline[T]) BindClock(clock timing.TimeTeller) {
	if clock == nil {
		panic("fast pipeline must have a clock")
	}

	p.clock = clock
}

// Size returns the number of resident items.
func (p *FastPipeline[T]) Size() int {
	return len(p.entries)
}

// currentCycle returns the clock's cycle. It panics without a clock or if the
// clock is earlier than the last synchronized cycle.
func (p *FastPipeline[T]) currentCycle() uint64 {
	if p.clock == nil {
		panic("fast pipeline has no clock; call BindClock after decoding")
	}

	now := p.freq.Cycle(p.clock.CurrentTime())
	if now < p.cycle {
		panic(fmt.Sprintf("fast pipeline cannot move from cycle %d back to %d", p.cycle, now))
	}

	return now
}

// elapsed returns the cycles that count toward progress since the last
// synchronization. It reads the clock without changing the pipeline.
func (p *FastPipeline[T]) elapsed() uint64 {
	if p.paused {
		return 0
	}

	return p.currentCycle() - p.cycle
}

// progressAfter returns e's progress once elapsed more cycles have counted.
func (p *FastPipeline[T]) progressAfter(e *fastPipelineEntry[T], elapsed uint64) int {
	if remaining := p.numStages - e.Progress; elapsed >= uint64(remaining) {
		return p.numStages
	}

	return e.Progress + int(elapsed)
}

// sync brings every item forward to the current cycle.
func (p *FastPipeline[T]) sync() {
	p.advance(p.elapsed())
}

// advance moves the pipeline and every item forward by elapsed cycles.
func (p *FastPipeline[T]) advance(elapsed uint64) {
	if elapsed == 0 {
		return
	}

	p.cycle += elapsed
	for i := range p.entries {
		e := &p.entries[i]
		e.Progress = p.progressAfter(e, elapsed)
	}
}

// Pause stops item progress at the end of the previous cycle, so the current
// cycle does not count. It has no effect on a paused pipeline.
func (p *FastPipeline[T]) Pause() {
	if p.paused {
		return
	}

	if now := p.currentCycle(); now > p.cycle {
		p.advance(now - 1 - p.cycle)
	}
	p.paused = true
}

// Resume discards the paused cycles, so the current cycle is the first that
// counts again. It has no effect on a pipeline that is not paused.
func (p *FastPipeline[T]) Resume() {
	if !p.paused {
		return
	}

	if now := p.currentCycle(); now > p.cycle {
		p.cycle = now - 1
	}
	p.paused = false
}

// CanPush returns true if some lane has stage 0 free in the current cycle.
func (p *FastPipeline[T]) CanPush() bool {
	lane, _ := p.admission()

	return lane >= 0
}

// Push inserts an item at stage 0 of the lowest free lane. It panics when no
// lane can accept; callers must check CanPush first.
func (p *FastPipeline[T]) Push(item T) {
	p.sync()

	lane, _ := p.admission()
	if lane < 0 {
		panic("fast pipeline has no free input lane")
	}

	p.entries = append(p.entries, fastPipelineEntry[T]{Lane: lane, Item: item})
}

// admission returns the lowest lane with stage 0 free now, or -1, and whether
// a lane that is busy only with this cycle's push frees up next cycle.
func (p *FastPipeline[T]) admission() (lane int, nextCycle bool) {
	if p.width <= 0 {
		return -1, false
	}
	if len(p.entries) == 0 {
		return 0, false
	}

	elapsed := p.elapsed()
	counts := p.scratch()
	for i := range p.entries {
		counts[p.entries[i].Lane]++
	}
	// An item with no progress holds stage 0 for this cycle only, unless its
	// lane is full.
	if elapsed == 0 {
		for i := range p.entries {
			e := &p.entries[i]
			if e.Progress == 0 && counts[e.Lane] < p.numStages {
				counts[e.Lane] = p.numStages
				nextCycle = true
			}
		}
	}

	for l, count := range counts {
		if count < p.numStages {
			return l, nextCycle
		}
	}

	return -1, nextCycle
}

// oldestReady returns the entry index of the oldest pushed ready lane head once
// elapsed more cycles have counted. Only the head of a lane can be ready, and a
// blocked lane does not hold back others.
func (p *FastPipeline[T]) oldestReady(elapsed uint64) (int, bool) {
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
		if p.progressAfter(e, elapsed) == p.numStages {
			return i, true
		}
	}

	return 0, false
}

// Peek returns the oldest pushed ready item without removing it. A Pop in the
// same cycle removes the same item.
func (p *FastPipeline[T]) Peek() (T, bool) {
	if len(p.entries) != 0 {
		if i, ok := p.oldestReady(p.elapsed()); ok {
			return p.entries[i].Item, true
		}
	}

	var zero T
	return zero, false
}

// UpdateReady replaces every ready lane head, oldest pushed first, with the
// result of update. Use it to keep completed work attached while backpressured.
// update runs again on a head that stays in place, so it must be idempotent,
// and it must not call back into the pipeline.
func (p *FastPipeline[T]) UpdateReady(update func(T) T) {
	if len(p.entries) == 0 {
		return
	}
	p.sync()

	seen := p.scratch()
	for i := range p.entries {
		e := &p.entries[i]
		if seen[e.Lane] != 0 {
			continue
		}
		seen[e.Lane] = 1
		if e.Progress == p.numStages {
			e.Item = update(e.Item)
		}
	}
}

// Pop removes and returns the oldest pushed ready item. Call it only after the
// output has accepted the item. The next item in its lane cannot become ready
// before the next cycle.
func (p *FastPipeline[T]) Pop() (T, bool) {
	if len(p.entries) == 0 {
		var zero T
		return zero, false
	}
	p.sync()

	return p.popReady()
}

// popReady removes the oldest ready head of a synchronized pipeline.
func (p *FastPipeline[T]) popReady() (T, bool) {
	i, ok := p.oldestReady(0)
	if !ok {
		var zero T
		return zero, false
	}

	item, lane := p.entries[i].Item, p.entries[i].Lane
	p.entries = slices.Delete(p.entries, i, i+1)
	for j := i; j < len(p.entries); j++ {
		if e := &p.entries[j]; e.Lane == lane {
			e.Progress = min(e.Progress, p.numStages-1)
			break
		}
	}

	return item, true
}

// DrainReady moves ready lane heads into sink, oldest pushed first, while it
// has room and reports whether any moved. A head that does not fit stays ready
// in its lane. Each lane delivers at most one item per cycle.
func (p *FastPipeline[T]) DrainReady(sink Sink[T]) bool {
	if len(p.entries) == 0 {
		return false
	}
	p.sync()

	moved := false
	for sink.CanPush() {
		item, ok := p.popReady()
		if !ok {
			break
		}

		sink.Push(item)
		moved = true
	}

	return moved
}

// NextReadyTime returns the time of the earliest future cycle at which a lane
// head becomes ready, assuming no further pops. Heads that are already ready
// are excluded, so a head waiting on backpressure does not hide another lane's
// later head. It returns false when no lane head is still on its way, and while
// paused, because no deadline can pass until Resume.
func (p *FastPipeline[T]) NextReadyTime() (timing.VTimeInPicoSec, bool) {
	if p.paused || len(p.entries) == 0 {
		return 0, false
	}

	elapsed := p.elapsed()
	seen := p.scratch()
	best := 0
	found := false
	for i := range p.entries {
		e := &p.entries[i]
		if seen[e.Lane] != 0 {
			continue
		}
		seen[e.Lane] = 1
		if wait := p.numStages - p.progressAfter(e, elapsed); wait > 0 && (!found || wait < best) {
			best, found = wait, true
		}
	}
	if !found {
		return 0, false
	}

	return p.cycleTime(p.cycle + elapsed + uint64(best)), true
}

// NextPushTime returns the earliest time at which CanPush becomes true without
// any further pop: the current cycle or the next one. It returns false when
// every lane is full, because only a pop can then release space; wait for
// NextReadyTime and pop instead. It also returns false while paused.
func (p *FastPipeline[T]) NextPushTime() (timing.VTimeInPicoSec, bool) {
	if p.paused {
		return 0, false
	}

	lane, nextCycle := p.admission()
	switch {
	case lane >= 0:
		return p.cycleTime(p.currentCycle()), true
	case nextCycle:
		return p.cycleTime(p.currentCycle() + 1), true
	default:
		return 0, false
	}
}

// cycleTime returns the time of the clock edge of cycle.
func (p *FastPipeline[T]) cycleTime(cycle uint64) timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(cycle) * p.freq.Period()
}

// Stages returns a copy of the resident items, oldest pushed first, each with
// its current lane and stage. It is intended for inspection and testing and
// does not change the pipeline. A decoded pipeline without a clock reports its
// saved positions.
func (p *FastPipeline[T]) Stages() []PipelineStage[T] {
	out := make([]PipelineStage[T], len(p.entries))
	if len(p.entries) == 0 {
		return out
	}

	var elapsed uint64
	if p.clock != nil {
		elapsed = p.elapsed()
	}

	ahead := p.scratch()
	for i := range p.entries {
		e := &p.entries[i]
		out[i] = PipelineStage[T]{
			Lane:  e.Lane,
			Stage: min(p.progressAfter(e, elapsed), p.numStages-1-ahead[e.Lane]),
			Item:  e.Item,
		}
		ahead[e.Lane]++
	}

	return out
}

// Clear removes all items. The pipeline keeps its clock position and any pause.
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
