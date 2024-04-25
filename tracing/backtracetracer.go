package tracing

import (
	"log"
	"sync"
)

// BackTraceTracer can record tasks incomplete tasks
type BackTraceTracer struct {
	tracingTasks map[string]Task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer() *BackTraceTracer {
	t := &BackTraceTracer{
		tracingTasks: make(map[string]Task),
	}

	return t
}

func (t *BackTraceTracer) StartTask(task Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.tracingTasks[task.ID] = task
}

func (t *BackTraceTracer) StepTask(task Task) {
	// Do Nothing
}

func (t *BackTraceTracer) EndTask(task Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.tracingTasks, task.ID)
}

func (t *BackTraceTracer) DumpBackTrace(task Task) {
	log.Printf("Task %s", task.ID)

	if task.ParentID == "" {
		return
	}

	t.DumpBackTrace(t.tracingTasks[task.ParentID])
}
