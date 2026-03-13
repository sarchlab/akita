package stateutil

// PipelineStage represents a single item occupying a lane and stage in a
// pipeline. It is JSON-serializable.
type PipelineStage[T any] struct {
	Lane      int `json:"lane"`
	Stage     int `json:"stage"`
	Item      T   `json:"item"`
	CycleLeft int `json:"cycle_left"`
}

// Pipeline is a generic multi-lane, multi-stage pipeline. It is a
// JSON-serializable value type.
type Pipeline[T any] struct {
	Width     int                 `json:"width"`
	NumStages int                 `json:"num_stages"`
	Stages    []PipelineStage[T]  `json:"stages"`
}

// CanAccept returns true if there is at least one free lane at stage 0.
func (p *Pipeline[T]) CanAccept() bool {
	occupied := 0
	for i := range p.Stages {
		if p.Stages[i].Stage == 0 {
			occupied++
		}
	}

	return occupied < p.Width
}

// Accept inserts an item into the first stage of the pipeline. It occupies
// the next free lane. The item starts with CycleLeft equal to NumStages-1
// (i.e., it needs that many ticks to reach the output).
func (p *Pipeline[T]) Accept(item T) {
	// Use a fixed-size bitset on the stack for small widths,
	// fall back to a slice for larger ones.
	var usedSmall [16]bool
	var used []bool

	if p.Width <= len(usedSmall) {
		used = usedSmall[:p.Width]
	} else {
		used = make([]bool, p.Width)
	}

	for i := range p.Stages {
		if p.Stages[i].Stage == 0 {
			used[p.Stages[i].Lane] = true
		}
	}

	lane := 0
	for lane < p.Width {
		if !used[lane] {
			break
		}
		lane++
	}

	p.Stages = append(p.Stages, PipelineStage[T]{
		Lane:      lane,
		Stage:     0,
		Item:      item,
		CycleLeft: p.NumStages - 1,
	})
}

// Tick advances the pipeline by one cycle. Items at the last stage with
// CycleLeft==0 are pushed into postBuf. Items at intermediate stages advance
// to the next stage if the next stage has a free lane. Stages are processed
// from last to first to prevent double-advancement.
//
// Returns true if any item moved.
func (p *Pipeline[T]) Tick(postBuf *Buffer[T]) bool {
	return p.TickFunc(func(item T) bool {
		if postBuf.CanPush() {
			postBuf.PushTyped(item)
			return true
		}
		return false
	})
}

// TickFunc advances the pipeline by one cycle, using the provided accept
// function for items that have completed all stages (CycleLeft==0 at last
// stage). The accept function should return true if it consumed the item.
//
// Returns true if any item moved.
func (p *Pipeline[T]) TickFunc(accept func(T) bool) bool {
	moved := false

	// Try to output items at last stage with CycleLeft == 0.
	// Process from end of slice so we can remove in-place.
	for i := len(p.Stages) - 1; i >= 0; i-- {
		s := &p.Stages[i]
		if s.Stage == p.NumStages-1 && s.CycleLeft == 0 {
			if accept(s.Item) {
				// Remove this entry.
				p.Stages = append(p.Stages[:i], p.Stages[i+1:]...)
				moved = true
			}
		}
	}

	// Build occupancy as a flat bool slice: occupied[stage*width + lane].
	// Use a stack-allocated array for small pipelines.
	totalSlots := p.NumStages * p.Width
	var occSmall [64]bool
	var occ []bool
	if totalSlots <= len(occSmall) {
		occ = occSmall[:totalSlots]
		// Zero out only what we use (the array is zeroed on allocation by Go,
		// but we need to be safe in case of reuse patterns).
		for i := range occ {
			occ[i] = false
		}
	} else {
		occ = make([]bool, totalSlots)
	}

	for i := range p.Stages {
		s := &p.Stages[i]
		occ[s.Stage*p.Width+s.Lane] = true
	}

	// Process from highest stage to lowest.
	for stage := p.NumStages - 2; stage >= 0; stage-- {
		for i := range p.Stages {
			s := &p.Stages[i]
			if s.Stage != stage {
				continue
			}

			if s.CycleLeft > 0 {
				s.CycleLeft--
				moved = true
				continue
			}

			// CycleLeft == 0, try to advance to next stage.
			nextStage := stage + 1
			if occ[nextStage*p.Width+s.Lane] {
				continue
			}

			// Advance.
			occ[s.Stage*p.Width+s.Lane] = false
			s.Stage = nextStage
			s.CycleLeft = 0 // Will be at next stage for one cycle.
			occ[nextStage*p.Width+s.Lane] = true
			moved = true
		}
	}

	return moved
}
