package hooking

import (
	"fmt"
	"sync"
)

// taskPrinter can print tasks with a format.
type taskPrinter interface {
	Print(task task)
}

type defaultTaskPrinter struct {
}

func (p *defaultTaskPrinter) Print(task task) {
	fmt.Printf("%s-%s@%s\n", task.Kind, task.What, task.Where)
}

// BackTraceTracer can record tasks incomplete tasks
type BackTraceTracer struct {
	printer      taskPrinter
	tracingTasks map[string]task
	lock         sync.Mutex
}

// NewBackTraceTracer creates a new BackTraceTracer
func NewBackTraceTracer(printer taskPrinter) *BackTraceTracer {
	t := &BackTraceTracer{
		printer:      printer,
		tracingTasks: make(map[string]task),
	}

	if t.printer == nil {
		t.printer = &defaultTaskPrinter{}
	}

	return t
}

func (t *BackTraceTracer) StartTask(ctx HookCtx) {
	taskStart := ctx.Item.(TaskStart)

	t.lock.Lock()
	defer t.lock.Unlock()

	currTask := task{
		ID:       taskStart.ID,
		Kind:     taskStart.Kind,
		What:     taskStart.What,
		Where:    ctx.Domain.Name(),
		ParentID: taskStart.ParentID,
	}

	t.tracingTasks[taskStart.ID] = currTask
}

func (t *BackTraceTracer) EndTask(ctx HookCtx) {
	taskEnd := ctx.Item.(TaskEnd)

	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.tracingTasks, taskEnd.ID)
}

func (t *BackTraceTracer) DumpBackTrace(taskID string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	currTask, ok := t.tracingTasks[taskID]

	for ok {
		t.printer.Print(currTask)

		taskID = currTask.ParentID
		currTask, ok = t.tracingTasks[taskID]
	}
}
