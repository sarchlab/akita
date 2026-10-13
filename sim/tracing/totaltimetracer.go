package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// TotalTimeTracer can collect the total time of executing a certain type of
// task. If the execution of two tasks overlaps, this tracer will simply add
// the two task processing time together.
type TotalTimeTracer struct {
	tasks.NopObserver

	filter    TaskFilter
	lock      sync.Mutex
	totalTime timing.VTimeInPicoSec
}

// NewTotalTimeTracer creates a new TotalTimeTracer
func NewTotalTimeTracer(filter TaskFilter) *TotalTimeTracer {
	return &TotalTimeTracer{
		filter: filter,
	}
}

// TotalTime returns the total time has been spent on a certain type of tasks.
func (t *TotalTimeTracer) TotalTime() timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.totalTime
}

// TaskEnded accounts for a completed task using its full timing record.
func (t *TotalTimeTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.filter != nil && !t.filter(task) {
		return
	}
	t.totalTime += task.EndTime - task.StartTime
}
