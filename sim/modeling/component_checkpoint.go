package modeling

import (
	"io"

	"github.com/sarchlab/akita/v5/internal/componentcheckpoint"
	"github.com/sarchlab/akita/v5/sim/timing"
)

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
	return componentcheckpoint.Write(w, state, guard)
}

// ReadCheckpoint restores a component's State and pending-event guard. Loading
// is best effort: a checkpoint from the same executable restores exactly, and
// one from a different build or configuration is used if it decodes. There is
// no version or configuration check; an error is returned only when decoding
// fails.
func ReadCheckpoint[T any](
	r io.Reader, state *T, guard PendingEventGuard,
) error {
	return componentcheckpoint.Read(r, state, guard)
}
