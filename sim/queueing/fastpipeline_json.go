package queueing

import (
	"encoding/json"
	"errors"
	"fmt"
)

// fastPipelineState is the JSON form of a FastPipeline: its geometry, its
// synchronized cycle, and its resident items in acceptance order. The per-lane
// scratch is derived and is not saved.
type fastPipelineState[T any] struct {
	Width     int                    `json:"width"`
	NumStages int                    `json:"num_stages"`
	Cycle     uint64                 `json:"cycle"`
	Entries   []fastPipelineEntry[T] `json:"entries"`
}

// MarshalJSON serializes the pipeline's geometry, cycle, and resident items.
func (p FastPipeline[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(fastPipelineState[T]{
		Width:     p.width,
		NumStages: p.numStages,
		Cycle:     p.cycle,
		Entries:   p.entries,
	})
}

// UnmarshalJSON restores a pipeline from its JSON form. It reports an error for
// a push Pipeline snapshot, whose "stages" would otherwise be dropped, and for
// entries that do not fit the saved geometry. On error p is unchanged.
func (p *FastPipeline[T]) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, legacy := fields["stages"]; legacy {
		return errors.New("fast pipeline: cannot restore a push Pipeline snapshot; " +
			"its in-flight items are not migrated")
	}

	var s fastPipelineState[T]
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	counts := make(map[int]int, min(len(s.Entries), max(s.Width, 0)))
	for _, e := range s.Entries {
		if e.Lane < 0 || e.Lane >= s.Width || e.Progress < 0 || e.Progress > s.NumStages {
			return fmt.Errorf("fast pipeline: entry at lane %d with progress %d "+
				"does not fit %d lanes and %d stages",
				e.Lane, e.Progress, s.Width, s.NumStages)
		}
		counts[e.Lane]++
		if counts[e.Lane] > s.NumStages {
			return fmt.Errorf("fast pipeline: lane %d holds more than %d items",
				e.Lane, s.NumStages)
		}
	}

	p.width = s.Width
	p.numStages = s.NumStages
	p.cycle = s.Cycle
	p.entries = s.Entries
	p.laneScratch = nil

	return nil
}
