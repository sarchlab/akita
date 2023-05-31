package tracing

import (
	"github.com/sarchlab/akita/v3/sim"

	"github.com/tebeka/atexit"
)

// TracerBackend is a backend that can store tasks.
type TracerBackend interface {
	// Write writes a task to the storage.
	Write(task Task)

	// Flush flushes the tasks to the storage, in case if the backend buffers
	// the tasks.
	Flush()
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.)
type DBTracer struct {
	timeTeller sim.TimeTeller
	backend    TracerBackend

	startTime, endTime sim.VTimeInSec

	tracingTasks map[string]Task
}

// StartTask marks the start of a task.
func (t *DBTracer) StartTask(task Task) {
	t.startingTaskMustBeValid(task)

	task.StartTime = t.timeTeller.CurrentTime()
	if t.endTime > 0 && task.StartTime > t.endTime {
		return
	}

	t.tracingTasks[task.ID] = task
}

func (t *DBTracer) startingTaskMustBeValid(task Task) {
	if task.ID == "" {
		panic("task ID must be set")
	}

	if task.Kind == "" {
		panic("task kind must be set")
	}

	if task.What == "" {
		panic("task what must be set")
	}

	if task.Where == "" {
		panic("task where must be set")
	}
}

// StepTask marks a step of a task.
func (t *DBTracer) StepTask(_ Task) {
	// Do nothing for now.
}

// EndTask marks the end of a task.
func (t *DBTracer) EndTask(task Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	if t.startTime > 0 && task.EndTime < t.startTime {
		delete(t.tracingTasks, task.ID)
		return
	}

	originalTask, ok := t.tracingTasks[task.ID]
	if !ok {
		return
	}

	originalTask.EndTime = task.EndTime
	delete(t.tracingTasks, task.ID)

	t.backend.Write(originalTask)
}

// Terminate terminates the tracer.
func (t *DBTracer) Terminate() {
	for _, task := range t.tracingTasks {
		task.EndTime = t.timeTeller.CurrentTime()
		t.backend.Write(task)
	}

	t.tracingTasks = nil

	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller sim.TimeTeller,
	backend TracerBackend,
) *DBTracer {
	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      backend,
		tracingTasks: make(map[string]Task),
	}

	atexit.Register(func() { t.Terminate() })

	return t
}

// SetTimeRange sets the time range of the tracer.
func (t *DBTracer) SetTimeRange(startTime, endTime sim.VTimeInSec) {
	t.startTime = startTime
	t.endTime = endTime
}
