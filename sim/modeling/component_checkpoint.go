package modeling

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// componentCheckpoint is the serialized form of a generic component: the
// mutable State and the pending-event guard. Spec, Resources and ports are
// rebuilt by setup, not serialized or compared. The guard is saved directly,
// alongside the engine's matching event, so load is a single pass with no
// post-load reconciliation against the queue.
type componentCheckpoint struct {
	State             json.RawMessage         `json:"state"`
	PendingEventGuard []timing.VTimeInPicoSec `json:"pending_event_guard"`
}

// PendingEventGuard records the times of the component's own pending ticks or
// wakeups, so restore does not schedule a duplicate.
type PendingEventGuard interface {
	// Snapshot returns the guard: the times of the pending events.
	Snapshot() []timing.VTimeInPicoSec

	// Restore sets the guard from a checkpoint.
	Restore(pending []timing.VTimeInPicoSec)
}

// WriteCheckpoint writes a component's State and pending-event guard as JSON.
// Component models use it to implement SaveCheckpoint; guard is nil for a
// component without its own scheduler.
func WriteCheckpoint[T any](
	w io.Writer, state T, guard PendingEventGuard,
) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	dto := componentCheckpoint{State: data}
	if guard != nil {
		dto.PendingEventGuard = guard.Snapshot()
	}

	return json.NewEncoder(w).Encode(dto)
}

// ReadCheckpoint restores a component's State and pending-event guard. Loading
// is best effort: a checkpoint from the same executable restores exactly, and
// one from a different build or configuration is used if it decodes. There is
// no version or configuration check; an error is returned only when decoding
// fails.
func ReadCheckpoint[T any](
	r io.Reader, state *T, guard PendingEventGuard,
) error {
	var dto componentCheckpoint
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
