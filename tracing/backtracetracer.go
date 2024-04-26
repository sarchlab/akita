package tracing

import (
	"fmt"
	"sync"
)

// TaskPrinter can print tasks with a format.
type TaskPrinter interface {
	Print(task Task)
}

type defaultTaskPrinter struct {
}

func (p *defaultTaskPrinter) Print(task Task) {
	fmt.Printf("%s-%s@%s\n", task.Kind, task.What, task.Where)
}

// BackTraceTracer can record tasks incomplete tasks
type BackTraceTracer struct {
	printer      TaskPrinter
	tracingTasks map[string]Task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer(printer TaskPrinter) *BackTraceTracer {
	t := &BackTraceTracer{
		printer:      printer,
		tracingTasks: make(map[string]Task),
	}

	if t.printer == nil {
		t.printer = &defaultTaskPrinter{}
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
	t.printer.Print(task)

	if task.ParentID == "" {
		return
	}

	t.DumpBackTrace(t.tracingTasks[task.ParentID])
}
