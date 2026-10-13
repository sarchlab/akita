package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// componentCheckpoint is the serialized form of a generic component: a spec hash
// for compatibility checking, the mutable State, and the pending-event guard.
// Resources and ports are rebuilt by setup, not serialized. The guard is saved
// directly — alongside the engine's matching event — so load is a single pass
// with no post-load reconciliation against the queue.
type componentCheckpoint struct {
	SpecHash          string                      `json:"spec_hash"`
	State             json.RawMessage             `json:"state"`
	PendingEventGuard pendingEventGuardCheckpoint `json:"pending_event_guard"`
}

// pendingEventGuardCheckpoint is the serialized pending-event guard: whether an event
// is pending and at what time.
type pendingEventGuardCheckpoint struct {
	Scheduled bool                  `json:"scheduled"`
	At        timing.VTimeInPicoSec `json:"at"`
}

// PendingEventGuard records whether the component's own tick or wakeup is
// pending and when, so restore does not schedule a duplicate.
type PendingEventGuard interface {
	// Snapshot returns the guard: whether an event is pending and at what time.
	Snapshot() (at timing.VTimeInPicoSec, scheduled bool)

	// Restore sets the guard from a checkpoint.
	Restore(at timing.VTimeInPicoSec, scheduled bool)
}

// WriteCheckpoint writes a component's spec hash, State, and pending-event guard
// as JSON. Component models use it to implement SaveCheckpoint; guard is nil for a
// component without its own scheduler.
func WriteCheckpoint[S, T any](
	w io.Writer, spec S, state T, guard PendingEventGuard,
) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	dto := componentCheckpoint{
		SpecHash: specHash(spec),
		State:    data,
	}
	if guard != nil {
		at, scheduled := guard.Snapshot()
		dto.PendingEventGuard = pendingEventGuardCheckpoint{Scheduled: scheduled, At: at}
	}

	return json.NewEncoder(w).Encode(dto)
}

// ReadCheckpoint restores a component's State and pending-event guard after
// verifying that the saved spec hash matches spec, the rebuilt component's.
func ReadCheckpoint[S, T any](
	r io.Reader, spec S, state *T, guard PendingEventGuard,
) error {
	var dto componentCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode component checkpoint: %w", err)
	}

	if got := specHash(spec); got != dto.SpecHash {
		return fmt.Errorf(
			"modeling: spec hash mismatch: checkpoint %s, rebuilt %s",
			dto.SpecHash, got)
	}

	var restored T
	if err := json.Unmarshal(dto.State, &restored); err != nil {
		return fmt.Errorf("modeling: unmarshal state: %w", err)
	}
	*state = restored

	if guard != nil {
		guard.Restore(dto.PendingEventGuard.At, dto.PendingEventGuard.Scheduled)
	}

	return nil
}

// specHash is a deterministic fingerprint of a component's immutable Spec, used
// to reject loading a checkpoint into a component built with a different config.
func specHash(spec any) string {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(fmt.Sprintf("modeling: cannot hash spec: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
