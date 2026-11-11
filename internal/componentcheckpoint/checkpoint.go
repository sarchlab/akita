// Package componentcheckpoint encodes component State and scheduler metadata together.
package componentcheckpoint

import (
	"encoding/json"
	"fmt"
	"io"
)

// componentCheckpoint is the serialized form of a generic component: the
// mutable State and the pending-event guard. Spec, Resources and ports are
// rebuilt by setup, not serialized or compared. The guard is saved directly,
// alongside the engine's matching event, so load is a single pass with no
// post-load reconciliation against the queue.
type componentCheckpoint[G any] struct {
	State             json.RawMessage `json:"state"`
	PendingEventGuard G               `json:"pending_event_guard"`
}

// Guard records the times of the component's own pending ticks or
// wakeups, so restore does not schedule a duplicate.
type Guard[G any] interface {
	// Snapshot returns the guard: the times of the pending events.
	Snapshot() G

	// Restore sets the guard from a checkpoint.
	Restore(pending G)
}

// Write writes a component's State and pending-event guard as JSON.
// Component models use it to implement SaveCheckpoint; guard is nil for a
// component without its own scheduler.
func Write[T, G any](
	w io.Writer, state T, guard Guard[G],
) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	dto := componentCheckpoint[G]{State: data}
	if guard != nil {
		dto.PendingEventGuard = guard.Snapshot()
	}

	return json.NewEncoder(w).Encode(dto)
}

// Read restores a component's State and pending-event guard. Loading
// is best effort: a checkpoint from the same executable restores exactly, and
// one from a different build or configuration is used if it decodes. There is
// no version or configuration check; an error is returned only when decoding
// fails.
func Read[T, G any](
	r io.Reader, state *T, guard Guard[G],
) error {
	var dto componentCheckpoint[G]
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode component checkpoint: %w", err)
	}

	var restored T
	if err := json.Unmarshal(dto.State, &restored); err != nil {
		return fmt.Errorf("modeling: unmarshal state: %w", err)
	}
	*state = restored

	if guard != nil {
		guard.Restore(dto.PendingEventGuard)
	}

	return nil
}
