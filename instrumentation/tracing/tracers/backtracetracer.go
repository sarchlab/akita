package tracers

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/instrumentation/tracing"
)

// TaskPrinter can print tasks with a format.
type TaskPrinter interface {
	Print(task tracing.Task)
}

type defaultTaskPrinter struct {
}

func (p *defaultTaskPrinter) Print(task tracing.Task) {
	fmt.Printf("%s-%s@%s\n", task.Kind, task.What, task.Location)
}

// BackTraceTracer can record tasks incomplete tasks
type BackTraceTracer struct {
	printer      TaskPrinter
	tracingTasks map[string]tracing.Task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer(printer TaskPrinter) *BackTraceTracer {
	t := &BackTraceTracer{
		printer:      printer,
		tracingTasks: make(map[string]tracing.Task),
	}

	if t.printer == nil {
		t.printer = &defaultTaskPrinter{}
	}

	return t
}

func (t *BackTraceTracer) StartTask(task tracing.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.tracingTasks[task.ID] = task
}

func (t *BackTraceTracer) TagTask(task tracing.Task) {
	// Do Nothing
}

// AddMilestone does nothing
func (t *BackTraceTracer) AddMilestone(_ tracing.Milestone) {
	// Do nothing
}

func (t *BackTraceTracer) EndTask(task tracing.Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.tracingTasks, task.ID)
}

func (t *BackTraceTracer) DumpBackTrace(task tracing.Task) {
	t.printer.Print(task)

	if task.ParentID == "" {
		return
	}

	parentTask, ok := t.tracingTasks[task.ParentID]
	if !ok {
		return
	}

	t.DumpBackTrace(parentTask)
}
