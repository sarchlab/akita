package hooking

import (
	"sync"
)

// TotalAvgTimeTracer can collect the total and average time of executing a
// certain type of task. If the execution of two tasks overlaps, this tracer
// will simply add the two task processing time together.
type TotalAvgTimeTracer struct {
	timeTeller    TimeTeller
	filter        TaskFilter
	lock          sync.Mutex
	inflightTasks map[string]task
	totalTime     float64
	taskCount     uint64
}

// NewAverageTimeTracer creates a new AverageTimeTracer
func NewAverageTimeTracer(
	timeTeller TimeTeller,
	filter TaskFilter,
) *TotalAvgTimeTracer {
	t := &TotalAvgTimeTracer{
		timeTeller:    timeTeller,
		filter:        filter,
		inflightTasks: make(map[string]task),
	}

	return t
}

// Func records the start end of a task.
func (t *TotalAvgTimeTracer) Func(ctx HookCtx) {
	switch ctx.Pos {
	case HookPosTaskStart:
		t.StartTask(ctx.Item.(TaskStart))
	case HookPosTaskEnd:
		t.EndTask(ctx.Item.(TaskEnd))
	}
}

// AverageTime returns the total time has been spent on a certain type of tasks.
func (t *TotalAvgTimeTracer) AverageTime() float64 {
	t.lock.Lock()
	time := t.totalTime / float64(t.taskCount)
	t.lock.Unlock()

	return time
}

// TotalCount returns the total number of tasks.
func (t *TotalAvgTimeTracer) TotalCount() uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskCount
}

// StartTask records the task start time
func (t *TotalAvgTimeTracer) StartTask(taskStart TaskStart) {
	if !t.filter(taskStart) {
		return
	}

	currTask := task{
		ID:        taskStart.ID,
		StartTime: t.timeTeller.Now(),
	}

	t.lock.Lock()
	t.inflightTasks[currTask.ID] = currTask
	t.lock.Unlock()
}

// EndTask records the end of the task
func (t *TotalAvgTimeTracer) EndTask(taskEnd TaskEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()

	currTask, ok := t.inflightTasks[taskEnd.ID]
	if !ok {
		return
	}

	endTime := t.timeTeller.Now()

	taskTime := endTime - currTask.StartTime

	t.totalTime += float64(taskTime)
	t.taskCount++

	delete(t.inflightTasks, currTask.ID)
}
