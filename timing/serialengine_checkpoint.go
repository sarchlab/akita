package timing

import (
	"encoding/json"
	"fmt"
	"io"
)

// serialEngineCheckpoint is the serialized form of the engine. The foundation
// supports quiescent checkpoints only, so it carries the current time; the event
// queues must be empty (serializing pending events needs the event codec
// registry, a later milestone).
type serialEngineCheckpoint struct {
	Time VTimeInSec `json:"time"`
}

// SaveCheckpoint writes the engine's current time. It refuses to checkpoint a
// non-empty event queue, since restoring pending events is not yet supported.
func (e *SerialEngine) SaveCheckpoint(w io.Writer) error {
	if e.queue.Len() != 0 || e.secondaryQueue.Len() != 0 {
		return fmt.Errorf(
			"timing: cannot checkpoint a non-empty serial engine queue " +
				"(non-quiescent checkpoints are not yet supported)")
	}

	return json.NewEncoder(w).Encode(serialEngineCheckpoint{Time: e.time})
}

// LoadCheckpoint restores the engine's current time into a freshly rebuilt
// engine, whose queues must already be empty.
func (e *SerialEngine) LoadCheckpoint(r io.Reader) error {
	if e.queue.Len() != 0 || e.secondaryQueue.Len() != 0 {
		return fmt.Errorf(
			"timing: cannot load a checkpoint into a non-empty serial engine queue")
	}

	var dto serialEngineCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}

	e.time = dto.Time
	return nil
}
