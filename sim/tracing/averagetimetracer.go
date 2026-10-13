package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// AverageTimeTracer can collect the average time of executing a certain type of
// task.
type AverageTimeTracer struct {
	tasks.NopObserver

	filter      TaskFilter
	lock        sync.Mutex
	averageTime timing.VTimeInPicoSec
	taskCount   uint64
}

// NewAverageTimeTracer creates a new AverageTimeTracer
func NewAverageTimeTracer(filter TaskFilter) *AverageTimeTracer {
	return &AverageTimeTracer{
		filter: filter,
	}
}

// AverageTime returns the average time spent on a certain type of tasks.
func (t *AverageTimeTracer) AverageTime() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.averageTime
}

// TotalCount returns the total number of tasks.
func (t *AverageTimeTracer) TotalCount() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskCount
}

// TaskEnded accounts for a completed task using its full timing record.
func (t *AverageTimeTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.filter != nil && !t.filter(task) {
		return
	}
	taskTime := task.EndTime - task.StartTime
	t.averageTime = (t.averageTime*timing.VTimeInPicoSec(t.taskCount) + taskTime) / timing.VTimeInPicoSec(t.taskCount+1)
	t.taskCount++
}
