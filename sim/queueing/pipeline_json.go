package queueing

import (
	"encoding/json"
	"sync/atomic"
)

// pipelineState is the JSON form of a Pipeline: its geometry and the items
// currently occupying it. Pipeline's fields are unexported, so without these
// methods encoding/json would serialize a Pipeline as an empty object and
// silently drop the in-flight items. PipelineStage is already all-exported, so
// the stages serialize directly.
type pipelineState[T any] struct {
	Width     int                `json:"width"`
	NumStages int                `json:"num_stages"`
	Stages    []PipelineStage[T] `json:"stages"`
}

// MarshalJSON serializes the pipeline's geometry and occupied stages.
func (p Pipeline[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(pipelineState[T]{
		Width:     p.width,
		NumStages: p.numStages,
		Stages:    p.stages,
	})
}

// UnmarshalJSON restores a pipeline from its JSON form.
func (p *Pipeline[T]) UnmarshalJSON(data []byte) error {
	var s pipelineState[T]
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	p.width = s.Width
	p.numStages = s.NumStages
	p.restoreStages(s.Stages)
	p.occupancy = nil

	return nil
}

// restoreStages replaces the stages with storage no other copy references and
// rebuilds the admission cache. Copies of the old stages are left undisturbed.
// Stages with invalid or aliased slots stay untracked so they keep the exact
// scanning behavior.
func (p *Pipeline[T]) restoreStages(stages []PipelineStage[T]) {
	p.stages = stages
	p.stageGen = nil
	p.seenGen = 0
	p.stageZero = 0

	if len(stages) > 0 && p.hasValidSlots() {
		p.stageGen = new(atomic.Uint64)
		p.stageZero = p.countStageZero()
	}
}

// hasValidSlots reports whether every item occupies its own in-range slot.
func (p *Pipeline[T]) hasValidSlots() bool {
	slots := make(map[[2]int]struct{}, len(p.stages))
	for i := range p.stages {
		s := &p.stages[i]
		if s.Lane < 0 || s.Lane >= p.width || s.Stage < 0 || s.Stage >= p.numStages {
			return false
		}

		slot := [2]int{s.Stage, s.Lane}
		if _, taken := slots[slot]; taken {
			return false
		}
		slots[slot] = struct{}{}
	}

	return true
}
