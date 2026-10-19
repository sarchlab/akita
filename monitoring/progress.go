package monitoring

import (
	"encoding/json"
	"sync"
	"time"
)

// ProgressBar tracks progress. Read a consistent copy with Snapshot and update
// counters through its methods; the live state and its lock are private.
type ProgressBar struct {
	mu   sync.Mutex
	data ProgressBarSnapshot
}

// ProgressBarSnapshot is a copy of a progress bar at one instant. Changing it
// does not change the live bar.
type ProgressBarSnapshot struct {
	ID         uint64    `json:"id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	Total      uint64    `json:"total"`
	Finished   uint64    `json:"finished"`
	InProgress uint64    `json:"in_progress"`
}

// Snapshot returns a consistent copy of the bar's state.
func (b *ProgressBar) Snapshot() ProgressBarSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data
}

// MarshalJSON preserves the progress API's fields without reading live counters
// outside their lock.
func (b *ProgressBar) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.Snapshot())
}

// IncrementInProgress adds the number of in-progress elements.
func (b *ProgressBar) IncrementInProgress(amount uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.InProgress += amount
}

// IncrementFinished adds a certain amount to finished elements.
func (b *ProgressBar) IncrementFinished(amount uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.Finished += amount
}

// MoveInProgressToFinished transfers the given number of items to finished.
// The caller must not finish more items than are in progress.
func (b *ProgressBar) MoveInProgressToFinished(amount uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data.InProgress -= amount
	b.data.Finished += amount
}
