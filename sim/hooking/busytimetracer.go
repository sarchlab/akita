package hooking

import (
	"container/list"
)

type taskTimeStartEnd struct {
	start, end float64
	completed  bool
}

// BusyTimeTracer traces the that a domain is processing a kind of task. If the
// task processing time overlaps, this tracer only consider one instance of the
// overlapped time.
type BusyTimeTracer struct {
	timeTeller    TimeTeller
	filter        TaskFilter
	inflightTasks map[string]*list.Element
	taskTimes     *list.List
	busyTime      float64
}

// NewBusyTimeTracer creates a new BusyTimeTracer
func NewBusyTimeTracer(
	timeTeller TimeTeller,
	filter TaskFilter,
) *BusyTimeTracer {
	t := &BusyTimeTracer{
		timeTeller:    timeTeller,
		filter:        filter,
		inflightTasks: make(map[string]*list.Element),
		taskTimes:     list.New(),
	}

	t.taskTimes.Init()

	return t
}

// Func records the start end of a task.
func (t *BusyTimeTracer) Func(ctx HookCtx) {
	switch ctx.Pos {
	case HookPosTaskStart:
		t.StartTask(ctx.Item.(TaskStart))
	case HookPosTaskEnd:
		t.EndTask(ctx.Item.(TaskEnd))
	}
}

// BusyTime returns the total time has been spent on a certain type of tasks.
func (t *BusyTimeTracer) BusyTime() float64 {
	return t.busyTime
}

// TerminateAllTasks will mark all the tasks as completed.
func (t *BusyTimeTracer) TerminateAllTasks() {
	now := t.timeTeller.Now()

	for e := t.taskTimes.Front(); e != nil; e = e.Next() {
		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			task.completed = true
			task.end = now
		}
	}

	t.collapse(now)
}

func (t *BusyTimeTracer) extendTaskTime(
	base *taskTimeStartEnd,
	t2 *taskTimeStartEnd,
) {
	if t2.start < base.start {
		base.start = t2.start
	}

	if t2.end > base.end {
		base.end = t2.end
	}
}

// StartTask records the task start time
func (t *BusyTimeTracer) StartTask(taskStart TaskStart) {
	if t.filter != nil && !t.filter(taskStart) {
		return
	}

	now := t.timeTeller.Now()
	taskTime := &taskTimeStartEnd{start: now}

	elem := t.taskTimes.PushBack(taskTime)
	t.inflightTasks[taskStart.ID] = elem
}

// EndTask records the end of the task
func (t *BusyTimeTracer) EndTask(taskEnd TaskEnd) {
	now := t.timeTeller.Now()

	originalTask, ok := t.inflightTasks[taskEnd.ID]
	if !ok {
		return
	}

	time := originalTask.Value.(*taskTimeStartEnd)
	time.end = now
	time.completed = true

	delete(t.inflightTasks, taskEnd.ID)

	t.collapse(now)
}

func (t *BusyTimeTracer) collapse(now float64) {
	time, found := t.startTimeOfFirstIncompleteTask()
	if found && time < now {
		return
	}

	finishedTasks := make([]*taskTimeStartEnd, 0)

	var next *list.Element
	for e := t.taskTimes.Front(); e != nil; e = next {
		next = e.Next()

		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			break
		}

		if task.completed && task.end <= now {
			finishedTasks = append(finishedTasks, task)

			t.taskTimes.Remove(e)
		}
	}

	t.busyTime += t.taskBusyTime(finishedTasks)
}

func (t *BusyTimeTracer) startTimeOfFirstIncompleteTask() (
	float64, bool,
) {
	for e := t.taskTimes.Front(); e != nil; e = e.Next() {
		task := e.Value.(*taskTimeStartEnd)
		if !task.completed {
			return task.start, true
		}
	}

	return 0, false
}

func (t *BusyTimeTracer) taskBusyTime(
	tasks []*taskTimeStartEnd,
) float64 {
	busyTime := 0.0
	coveredMask := make(map[int]bool)

	for i, t1 := range tasks {
		if _, covered := coveredMask[i]; covered {
			continue
		}

		coveredMask[i] = true

		extTime := taskTimeStartEnd{
			start: t1.start,
			end:   t1.end,
		}

		for j, t2 := range tasks {
			if _, covered := coveredMask[j]; covered {
				continue
			}

			if t.taskTimeOverlap(t1, t2) {
				coveredMask[j] = true

				t.extendTaskTime(&extTime, t2)
			}
		}

		busyTime += extTime.end - extTime.start
	}

	return busyTime
}

func (t *BusyTimeTracer) taskTimeOverlap(t1, t2 *taskTimeStartEnd) bool {
	if t1.start <= t2.start && t1.end >= t2.start {
		return true
	}

	if t1.start <= t2.end && t1.end >= t2.end {
		return true
	}

	if t1.start >= t2.start && t1.end <= t2.end {
		return true
	}

	return false
}
