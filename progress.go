package monitoring

import (
	"sync"
	"time"
)

// A ProgressBar is a tracker of the progress
type ProgressBar struct {
	sync.Mutex
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	StartTime  time.Time `json:"start_time"`
	Total      uint64    `json:"total"`
	Finished   uint64    `json:"finished"`
	InProgress uint64    `json:"in_progress"`
}

// IncrementInProgress adds the number of in-progress element.
func (b *ProgressBar) IncrementInProgress(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.InProgress += amount
}

// IncrementFinished add a certain amount to finished element.
func (b *ProgressBar) IncrementFinished(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.Finished += amount
}

// MoveInProgressToFinished reduces the number of in progress item by a certain
// amount and increase the finished item by the same amount.
func (b *ProgressBar) MoveInProgressToFinished(amount uint64) {
	b.Lock()
	defer b.Unlock()

	b.InProgress -= amount
	b.Finished += amount
}
