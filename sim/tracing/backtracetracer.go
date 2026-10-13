package tracing

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v5/sim/tasks"
)

// TaskPrinter can print tasks with a format.
type TaskPrinter interface {
	Print(task tasks.Task)
}

type defaultTaskPrinter struct {
}

func (p *defaultTaskPrinter) Print(task tasks.Task) {
	fmt.Printf("%s-%s@%s\n", task.Kind, task.What, task.Location)
}

// BackTraceTracer can record incomplete tasks.
type BackTraceTracer struct {
	tasks.NopObserver

	printer      TaskPrinter
	tracingTasks map[uint64]tasks.Task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer(printer TaskPrinter) *BackTraceTracer {
	t := &BackTraceTracer{
		printer:      printer,
		tracingTasks: make(map[uint64]tasks.Task),
	}

	if t.printer == nil {
		t.printer = &defaultTaskPrinter{}
	}

	return t
}

// TaskStarted records the started task so it can be back-traced.
func (t *BackTraceTracer) TaskStarted(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.tracingTasks[task.ID] = task
}

// TaskEnded removes the completed task.
func (t *BackTraceTracer) TaskEnded(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.tracingTasks, task.ID)
}

// DumpBackTrace prints the task and the chain of its ancestors.
func (t *BackTraceTracer) DumpBackTrace(task tasks.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.dumpBackTrace(task)
}

func (t *BackTraceTracer) dumpBackTrace(task tasks.Task) {
	t.printer.Print(task)

	if task.ParentID == 0 {
		return
	}

	parentTask, ok := t.tracingTasks[task.ParentID]
	if !ok {
		return
	}

	t.dumpBackTrace(parentTask)
}
