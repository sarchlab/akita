package queueing

import "encoding/json"

// pipelineStage represents a single item occupying a lane and stage in a
// pipeline. It is JSON-serializable.
type pipelineStage[T any] struct {
	Lane      int `json:"lane"`
	Stage     int `json:"stage"`
	Item      T   `json:"item"`
	CycleLeft int `json:"cycle_left"`
}

// Pipeline is a generic multi-lane, multi-stage pipeline. It is a
// JSON-serializable value type.
type Pipeline[T any] struct {
	Width        int `json:"width"`
	NumStages    int `json:"num_stages"`
	StageLatency int `json:"stage_latency"`

	stages []pipelineStage[T]
}

type pipelineJSON[T any] struct {
	Width        int                `json:"width"`
	NumStages    int                `json:"num_stages"`
	StageLatency int                `json:"stage_latency"`
	Stages       []pipelineStage[T] `json:"stages"`
}

// MarshalJSON serializes the pipeline, including its unexported internal
// stage state.
func (p Pipeline[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(pipelineJSON[T]{
		Width:        p.Width,
		NumStages:    p.NumStages,
		StageLatency: p.StageLatency,
		Stages:       p.stages,
	})
}

// UnmarshalJSON restores the pipeline, including its internal stage state.
func (p *Pipeline[T]) UnmarshalJSON(data []byte) error {
	var state pipelineJSON[T]
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	p.Width = state.Width
	p.NumStages = state.NumStages
	p.StageLatency = state.StageLatency
	p.stages = state.Stages

	return nil
}

// Len returns the number of items currently in the pipeline.
func (p *Pipeline[T]) Len() int {
	return len(p.stages)
}

// Clear removes all items from the pipeline.
func (p *Pipeline[T]) Clear() {
	p.stages = nil
}

// CanAccept returns true if there is at least one free lane at stage 0.
func (p *Pipeline[T]) CanAccept() bool {
	occupied := 0
	for i := range p.stages {
		if p.stages[i].Stage == 0 {
			occupied++
		}
	}

	return occupied < p.Width
}

// Accept inserts an item into the first stage of the pipeline. It occupies
// the next free lane. The item spends StageLatency cycles in each pipeline
// stage. A StageLatency of 0 is treated as 1 for compatibility with older
// serialized state.
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

	for i := range p.stages {
		if p.stages[i].Stage == 0 {
			used[p.stages[i].Lane] = true
		}
	}

	lane := 0
	for lane < p.Width {
		if !used[lane] {
			break
		}
		lane++
	}

	p.stages = append(p.stages, pipelineStage[T]{
		Lane:      lane,
		Stage:     0,
		Item:      item,
		CycleLeft: p.initialCycleLeft(),
	})
}

// Tick advances the pipeline by one cycle. Items at the last stage with
// CycleLeft==0 are pushed into postBuf. Items with CycleLeft>0 count down.
// Items at intermediate stages advance to the next stage if the next stage has
// a free lane. Stages are processed from last to first to prevent
// double-advancement.
//
// Returns true if any item moved.
func (p *Pipeline[T]) Tick(postBuf *Buffer[T]) bool {
	n := len(p.stages)
	if n == 0 {
		return false
	}

	moved := false
	lastStage := p.NumStages - 1

	// Phase 1: Try to output items at last stage with CycleLeft == 0.
	for i := n - 1; i >= 0; i-- {
		s := &p.stages[i]
		if s.Stage != lastStage {
			continue
		}

		if s.CycleLeft > 0 {
			s.CycleLeft--
			moved = true

			continue
		}

		if postBuf.CanPush() {
			postBuf.PushTyped(s.Item)
			p.stages[i] = p.stages[n-1]
			n--
			moved = true
		}
	}

	p.stages = p.stages[:n]

	// Phase 2: Advance remaining items from high stage to low.
	if n > 0 {
		moved = p.advanceItems() || moved
	}

	return moved
}

// advanceItems processes all pipeline items, advancing them toward higher
// stages. Items are processed from highest stage to lowest to prevent
// double-advancement within a single tick.
func (p *Pipeline[T]) advanceItems() bool {
	n := len(p.stages)
	moved := false

	minStage, maxStage := p.stageRange()

	// Cap maxStage: items at lastStage were already handled in Phase 1.
	lastStage := p.NumStages - 1
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

			if s.CycleLeft > 0 {
				s.CycleLeft--
				moved = true

				continue
			}

			nextStage := stage + 1
			nextIdx := (nextStage - occBase) * p.Width
			curIdx := (stage - occBase) * p.Width

			if occ[nextIdx+s.Lane] {
				continue
			}

			occ[curIdx+s.Lane] = false
			s.Stage = nextStage
			s.CycleLeft = p.initialCycleLeft()
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
	occSlots := occRange * p.Width

	var occSmall [128]bool
	var occ []bool

	if occSlots <= len(occSmall) {
		occ = occSmall[:occSlots]
		for i := range occ {
			occ[i] = false
		}
	} else {
		occ = make([]bool, occSlots)
	}

	for i := 0; i < len(p.stages); i++ {
		s := &p.stages[i]
		occ[(s.Stage-minStage)*p.Width+s.Lane] = true
	}

	return occ
}

func (p *Pipeline[T]) initialCycleLeft() int {
	latency := p.effectiveStageLatency()
	if latency <= 1 {
		return 0
	}

	return latency - 1
}

func (p *Pipeline[T]) effectiveStageLatency() int {
	if p.StageLatency <= 0 {
		return 1
	}

	return p.StageLatency
}
