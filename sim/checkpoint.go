package sim

import (
	"bytes"
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// SaveCheckpoint writes a checkpoint archive for the simulation. The simulation
// must use a SerialEngine with no active Run or RunUntil call, including paused
// execution. Execution cannot start until the checkpoint finishes.
// It returns an error for the first registered entity that cannot be saved.
func (s *Simulation) SaveCheckpoint(path string) error {
	return s.whileIdle(func() error { return s.saveCheckpoint(path) })
}

func (s *Simulation) saveCheckpoint(path string) error {
	if err := s.checkpointPreflight(); err != nil {
		return err
	}

	payloads := make([]archiveEntry, 0, len(s.entities))
	for _, entity := range s.entities {
		checkpointable, ok := entity.(Checkpointable)
		if !ok {
			return fmt.Errorf(
				"checkpoint: entity %q (%T) has no checkpoint serializer",
				entity.Name(), entity)
		}

		var buf bytes.Buffer
		if err := checkpointable.SaveCheckpoint(&buf); err != nil {
			return fmt.Errorf("checkpoint: save entity %q: %w", entity.Name(), err)
		}
		payloads = append(payloads,
			archiveEntry{name: entity.Name(), data: buf.Bytes()})
	}

	return writeArchive(path, payloads)
}

// LoadCheckpoint loads a checkpoint archive into this rebuilt simulation. It
// checks that the saved entity set matches the rebuilt one, then hands each
// entity its payload. Only the task tracker is optional: off->on starts it
// empty; on->off ignores its payload. Rebuild fresh observers. Code and serialized
// state must be compatible; decoding does not guarantee semantic compatibility.
func (s *Simulation) LoadCheckpoint(path string) error {
	return s.whileIdle(func() error { return s.loadCheckpoint(path) })
}

func (s *Simulation) loadCheckpoint(path string) error {
	if err := s.checkpointPreflight(); err != nil {
		return err
	}

	payloads, err := readArchive(path)
	if err != nil {
		return err
	}
	if err := s.checkpointCoverage(payloads); err != nil {
		return err
	}

	for _, entity := range s.entities {
		checkpointable, ok := entity.(Checkpointable)
		if !ok {
			return fmt.Errorf(
				"checkpoint: entity %q (%T) has no checkpoint serializer",
				entity.Name(), entity)
		}

		data, found := payloads[entity.Name()]
		if _, optional := entity.(*taskTrackerCheckpoint); optional && !found {
			data = []byte("{}") // Tracking was off when the checkpoint was saved.
		}
		if err := checkpointable.LoadCheckpoint(
			bytes.NewReader(data),
		); err != nil {
			return fmt.Errorf("checkpoint: load entity %q: %w", entity.Name(), err)
		}
	}

	return nil
}

func (s *Simulation) checkpointPreflight() error {
	if err := s.setup().Ready(); err != nil {
		return err
	}
	if _, ok := s.engine.(*timing.SerialEngine); !ok {
		return fmt.Errorf("checkpoint: only timing.SerialEngine is supported, got %T",
			s.engine)
	}

	return nil
}

// checkpointCoverage requires an exact match except for the optional task tracker.
func (s *Simulation) checkpointCoverage(payloads map[string][]byte) error {
	rebuilt := make(map[string]struct{}, len(s.entities))
	for _, entity := range s.entities {
		rebuilt[entity.Name()] = struct{}{}
	}

	for name := range payloads {
		if _, found := rebuilt[name]; !found && name != taskTrackerCheckpointName {
			return fmt.Errorf("checkpoint: saved entity %q is not rebuilt", name)
		}
	}
	for name := range rebuilt {
		if _, found := payloads[name]; !found && name != taskTrackerCheckpointName {
			return fmt.Errorf("checkpoint: rebuilt entity %q is missing from checkpoint",
				name)
		}
	}

	return nil
}
