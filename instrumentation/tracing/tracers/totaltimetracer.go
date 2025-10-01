package tracers

import (
	"sync"

	"github.com/sarchlab/akita/v4/instrumentation/tracing"
	"github.com/sarchlab/akita/v4/sim"
)

// TotalTimeTracer can collect the total time of executing a certain type of
// task. If the execution of two tasks overlaps, this tracer will simply add
// the two task processing time together.
type TotalTimeTracer struct {
	timeTeller    sim.TimeTeller
	filter        tracing.TaskFilter
	lock          sync.Mutex
	totalTime     sim.VTimeInSec
	inflightTasks map[string]tracing.Task
}

// NewTotalTimeTracer creates a new TotalTimeTracer
func NewTotalTimeTracer(
	timeTeller sim.TimeTeller,
	filter tracing.TaskFilter,
) *TotalTimeTracer {
	t := &TotalTimeTracer{
		timeTeller:    timeTeller,
		filter:        filter,
		inflightTasks: make(map[string]tracing.Task),
	}

	return t
}

// TotalTime returns the total time has been spent on a certain type of tasks.
func (t *TotalTimeTracer) TotalTime() sim.VTimeInSec {
	t.lock.Lock()
	time := t.totalTime
	t.lock.Unlock()

	return time
}

// StartTask records the task start time
func (t *TotalTimeTracer) StartTask(task tracing.Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task
	t.lock.Unlock()
}

// TagTask does nothing
func (t *TotalTimeTracer) TagTask(_ tracing.Task) {
	// Do nothing
}

// AddMilestone does nothing
func (t *TotalTimeTracer) AddMilestone(_ tracing.Milestone) {
	// Do nothing
}

// EndTask records the end of the task
func (t *TotalTimeTracer) EndTask(task tracing.Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	t.lock.Lock()

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		t.lock.Unlock()
		return
	}

	t.totalTime += task.EndTime - originalTask.StartTime
	delete(t.inflightTasks, task.ID)
	t.lock.Unlock()
}
