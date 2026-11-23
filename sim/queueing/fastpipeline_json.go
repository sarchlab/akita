package queueing

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// fastPipelineState is the JSON form of a FastPipeline: its geometry and
// frequency, its synchronized cycle and pause, and its resident items in push
// order. The clock is bound at runtime and the per-lane scratch is derived, so
// neither is saved.
type fastPipelineState[T any] struct {
	Width     int                    `json:"width"`
	NumStages int                    `json:"num_stages"`
	Freq      timing.Freq            `json:"freq"`
	Cycle     uint64                 `json:"cycle"`
	Paused    bool                   `json:"paused"`
	Entries   []fastPipelineEntry[T] `json:"entries"`
}

// MarshalJSON serializes the pipeline's geometry, timing, and resident items.
// It does not read the clock: items keep the position of the last
// synchronization and catch up once the restored pipeline has a clock.
func (p FastPipeline[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(fastPipelineState[T]{
		Width:     p.width,
		NumStages: p.numStages,
		Freq:      p.freq,
		Cycle:     p.cycle,
		Paused:    p.paused,
		Entries:   p.entries,
	})
}

// UnmarshalJSON restores a pipeline from its JSON form and leaves its clock as
// it was. It reports an error for a push Pipeline snapshot, whose "stages" would
// otherwise be dropped, for a configured pipeline without a frequency, and for
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
	if s.NumStages > 0 && s.Freq == 0 {
		return errors.New("fast pipeline: snapshot has no frequency")
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
	p.freq = s.Freq
	p.cycle = s.Cycle
	p.paused = s.Paused
	p.entries = s.Entries
	p.laneScratch = nil

	return nil
}
