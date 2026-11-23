package queueing

import "sync/atomic"

// PipelineStage represents a single item occupying a lane and stage in a
// pipeline. It is a plain data record used to inspect pipeline occupancy.
type PipelineStage[T any] struct {
	Lane  int `json:"lane"`
	Stage int `json:"stage"`
	Item  T   `json:"item"`
}

// Sink is the destination a Pipeline pushes completed items into. Both
// queueing.Buffer and any bounded collection that accepts items satisfy it.
type Sink[T any] interface {
	CanPush() bool
	Push(T)
}

// Pipeline is a generic multi-lane, multi-stage pipeline. Its state is
// encapsulated; callers drive it through Accept/Tick and inspect it through
// the accessors.
type Pipeline[T any] struct {
	width     int
	numStages int
	stages    []PipelineStage[T]
	// stageGen counts writes to the stages storage and is shared by Go copies
	// of this value. It is nil until the first insertion or restore, and stays
	// nil for restored stages that do not form valid slots.
	stageGen *atomic.Uint64
	// seenGen is the stageGen value after this copy's latest completed write. A
	// copy that falls behind may view slots another copy rewrote, so it stops
	// trusting stageZero and the fast advance until Clear or restore replaces
	// its stages. Tick updates it only after its sink calls return normally.
	seenGen uint64
	// stageZero counts the items at stage 0 while this copy is tracked. It is
	// derived from stages and is excluded from checkpoints.
	stageZero int
	// occupancy is scratch space rebuilt from stages on every advance. It is
	// not simulation state and is excluded from checkpoints.
	occupancy []bool
}

// MakePipeline creates a pipeline with the given width (lanes) and number of
// stages. It returns a value so the pipeline can be embedded directly in a
// component's state.
func MakePipeline[T any](width, numStages int) Pipeline[T] {
	return Pipeline[T]{
		width:     width,
		numStages: numStages,
	}
}

// Stages returns a copy of the items currently occupying the pipeline. It is
// intended for inspection and testing; mutating the returned slice has no
// effect on the pipeline.
func (p *Pipeline[T]) Stages() []PipelineStage[T] {
	out := make([]PipelineStage[T], len(p.stages))
	copy(out, p.stages)

	return out
}

// Clear removes all items from the pipeline.
func (p *Pipeline[T]) Clear() {
	p.stages = nil
	p.stageGen = nil
	p.seenGen = 0
	p.stageZero = 0
	p.occupancy = nil
}

// tracked reports whether no other copy has written the stages since this
// copy last did, so stageZero and the slot invariants can be relied on.
func (p *Pipeline[T]) tracked() bool {
	return p.stageGen != nil && p.seenGen == p.stageGen.Load()
}

// recordStageWrite tells other copies that the shared stages have changed. A
// copy that was already behind stays untracked.
func (p *Pipeline[T]) recordStageWrite(tracked bool) {
	if p.stageGen == nil {
		return
	}

	gen := p.stageGen.Add(1)
	if tracked {
		p.seenGen = gen
	}
}

// invalidateStages ensures no copy trusts its cache across a stage write,
// without updating seenGen. Given the tracker and version Tick last invalidated,
// it skips the update while that invalidation still holds for this value, and
// returns the tracker and version now in effect.
func (p *Pipeline[T]) invalidateStages(
	gen *atomic.Uint64, version uint64,
) (*atomic.Uint64, uint64) {
	if p.stageGen == nil {
		return nil, 0
	}
	if p.stageGen == gen && p.seenGen != version && gen.Load() == version {
		return gen, version
	}

	return p.stageGen, p.stageGen.Add(1)
}

// CanAccept returns true if there is at least one free lane at stage 0.
func (p *Pipeline[T]) CanAccept() bool {
	if p.tracked() {
		return p.stageZero < p.width
	}

	return p.countStageZero() < p.width
}

// Accept inserts an item into the first stage of the pipeline. It occupies
// the next free lane and attempts to advance to the next stage on the very
// next Tick. Each stage takes one cycle, so the total latency through the
// pipeline equals NumStages ticks. Accept panics when stage 0 is full; callers
// must check CanAccept before inserting.
func (p *Pipeline[T]) Accept(item T) {
	// Without backing storage, no other copy can alias this insertion.
	if p.stageGen == nil && cap(p.stages) == 0 {
		p.stageGen = new(atomic.Uint64)
		p.seenGen = 0
		p.stageZero = 0
	}

	tracked := p.tracked()
	if tracked && p.stageZero >= p.width {
		panic("pipeline has no free input lane")
	}

	lane := 0
	if !tracked || p.stageZero > 0 {
		lane = p.firstFreeLane(tracked)
	}
	if lane == p.width {
		panic("pipeline has no free input lane")
	}

	p.stages = append(p.stages, PipelineStage[T]{
		Lane:  lane,
		Stage: 0,
		Item:  item,
	})
	if tracked {
		p.stageZero++
	}
	p.recordStageWrite(tracked)
}

// firstFreeLane returns the lowest lane that is free at stage 0, or p.width
// when every lane is occupied.
func (p *Pipeline[T]) firstFreeLane(tracked bool) int {
	// Use a fixed-size bitset on the stack for small widths,
	// fall back to a slice for larger ones.
	var usedSmall [16]bool
	var used []bool

	if p.width <= len(usedSmall) {
		used = usedSmall[:p.width]
	} else {
		used = make([]bool, p.width)
	}

	// Newly accepted items sit at the back, so a tracked pipeline can stop once
	// all are found.
	left := len(p.stages)
	if tracked {
		left = p.stageZero
	}
	for i := len(p.stages) - 1; i >= 0 && left > 0; i-- {
		if p.stages[i].Stage == 0 {
			used[p.stages[i].Lane] = true
			left--
		}
	}

	lane := 0
	for lane < p.width && used[lane] {
		lane++
	}

	return lane
}

// Tick advances the pipeline by one cycle. Items at the last stage are pushed
// into sink. Items at intermediate stages advance
// to the next stage if the next stage has a free lane. Stages are processed
// from last to first to prevent double-advancement.
//
// Returns true if any item moved.
func (p *Pipeline[T]) Tick(sink Sink[T]) bool {
	n := len(p.stages)
	if n == 0 {
		return false
	}

	// exclusive holds while this Tick's own write is the only one since it began.
	exclusive := p.tracked()
	startGen, startSeen := p.stageGen, p.seenGen
	var writeGen *atomic.Uint64
	writeVersion := startSeen
	moved := false
	blocked := false
	lastStage := p.numStages - 1

	// Phase 1: Try to output items at the last stage.
	for i := n - 1; i >= 0; i-- {
		s := &p.stages[i]
		if s.Stage != lastStage {
			continue
		}
		if sink.CanPush() {
			sink.Push(s.Item)
			// A later sink call can observe or interrupt this swap, and the last
			// one may have replaced this value, so invalidate the current storage
			// first and leave seenGen behind until Tick returns.
			writeGen, writeVersion = p.invalidateStages(writeGen, writeVersion)
			exclusive = exclusive && writeGen == startGen && writeVersion == startSeen+1
			p.stages[i] = p.stages[n-1]
			n--
			moved = true
		} else {
			blocked = true
		}
	}

	p.stages = p.stages[:n]

	// A sink call may have replaced this value or written through a copy.
	intact := p.stageGen == startGen && p.seenGen == startSeen
	tracked := exclusive && intact && startGen.Load() == writeVersion

	moved = p.advanceStages(tracked, blocked) || moved
	if moved || !intact {
		writeGen, writeVersion = p.invalidateStages(writeGen, writeVersion)
	}
	if tracked {
		p.seenGen = writeVersion
	}

	return moved
}

// advanceStages runs the second phase of Tick. With the last stage empty, every
// remaining item of a tracked pipeline has a free slot ahead. Otherwise a
// blocked output or aliased slot can stall items, so advance from high stage to
// low. A pipeline without stages never advances.
func (p *Pipeline[T]) advanceStages(tracked, blocked bool) bool {
	if tracked && !blocked && p.numStages > 0 {
		for i := range p.stages {
			p.stages[i].Stage++
		}
		p.stageZero = 0

		return len(p.stages) > 0
	}

	if len(p.stages) == 0 {
		return false
	}

	moved := p.advanceItems()
	if tracked {
		p.stageZero = p.countStageZero()
	}

	return moved
}

// countStageZero returns the number of items at stage 0.
func (p *Pipeline[T]) countStageZero() int {
	count := 0
	for i := range p.stages {
		if p.stages[i].Stage == 0 {
			count++
		}
	}

	return count
}

// advanceItems processes all pipeline items, advancing them toward higher
// stages. Items are processed from highest stage to lowest to prevent
// double-advancement within a single tick.
func (p *Pipeline[T]) advanceItems() bool {
	n := len(p.stages)
	moved := false

	minStage, maxStage := p.stageRange()

	// Cap maxStage: items at lastStage were already handled in Phase 1.
	lastStage := p.numStages - 1
	if maxStage > lastStage-1 {
		maxStage = lastStage - 1
	}

	if maxStage < minStage {
		return moved
	}

	occ := p.buildOccupancy(minStage, maxStage+1)
	occBase := minStage

	for stage := maxStage; stage >= minStage; stage-- {
		for i := 0; i < n; i++ {
			s := &p.stages[i]
			if s.Stage != stage {
				continue
			}

			nextStage := stage + 1
			nextIdx := (nextStage - occBase) * p.width
			curIdx := (stage - occBase) * p.width

			if occ[nextIdx+s.Lane] {
				continue
			}

			occ[curIdx+s.Lane] = false
			s.Stage = nextStage
			occ[nextIdx+s.Lane] = true
			moved = true
		}
	}

	return moved
}

// stageRange returns the minimum and maximum stage numbers among items.
func (p *Pipeline[T]) stageRange() (int, int) {
	minStage := p.stages[0].Stage
	maxStage := p.stages[0].Stage

	for i := 1; i < len(p.stages); i++ {
		st := p.stages[i].Stage
		if st < minStage {
			minStage = st
		}

		if st > maxStage {
			maxStage = st
		}
	}

	return minStage, maxStage
}

// buildOccupancy builds a flat bool slice tracking which (stage, lane) slots
// are occupied, for stages in [minStage, maxStage+1].
func (p *Pipeline[T]) buildOccupancy(minStage, maxStage int) []bool {
	occRange := maxStage + 1 - minStage + 1 // +1 for nextStage lookups
	occSlots := occRange * p.width

	if cap(p.occupancy) < occSlots {
		p.occupancy = make([]bool, occSlots)
	}
	occ := p.occupancy[:occSlots]
	clear(occ)

	for i := 0; i < len(p.stages); i++ {
		s := &p.stages[i]
		occ[(s.Stage-minStage)*p.width+s.Lane] = true
	}

	return occ
}
