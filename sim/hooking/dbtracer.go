package hooking

import (
	"github.com/tebeka/atexit"
)

// TracerBackend is a backend that can store tasks.
type TracerBackend interface {
	// Write writes a task to the storage.
	Write(t task)

	// Flush flushes the tasks to the storage, in case if the backend buffers
	// the tasks.
	Flush()
}

// DBTracer is a tracer that can store tasks into a database.
type DBTracer struct {
	timeTeller         TimeTeller
	backend            TracerBackend
	startTime, endTime float64
	tracingTasks       map[string]task
}

// Func records the start end of a task.
func (t *DBTracer) Func(ctx HookCtx) {
	switch ctx.Pos {
	case HookPosTaskStart:
		t.StartTask(ctx.Item.(TaskStart))
	case HookPosTaskStep:
		t.StepTask(ctx.Item.(TaskStep))
	case HookPosTaskTag:
		t.TagTask(ctx.Item.(TaskTag))
	case HookPosTaskEnd:
		t.EndTask(ctx.Item.(TaskEnd))
	}
}

// StartTask marks the start of a task.
func (t *DBTracer) StartTask(taskStart TaskStart) {
	t.startingTaskMustBeValid(taskStart)

	currTask := task{
		ID:        taskStart.ID,
		ParentID:  taskStart.ParentID,
		Kind:      taskStart.Kind,
		What:      taskStart.What,
		StartTime: t.timeTeller.Now(),
	}

	if t.endTime > 0 && currTask.StartTime > t.endTime {
		return
	}

	t.tracingTasks[currTask.ID] = currTask
}

func (t *DBTracer) startingTaskMustBeValid(task TaskStart) {
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
func (t *DBTracer) StepTask(ts TaskStep) {
	originalTask, ok := t.tracingTasks[ts.TaskID]
	if !ok {
		return
	}

	originalTask.Steps = append(originalTask.Steps, step{
		ID:     ts.StepID,
		Time:   t.timeTeller.Now(),
		Kind:   ts.Kind,
		What:   ts.What,
		Detail: ts.Detail,
	})

	t.tracingTasks[ts.TaskID] = originalTask
}

// TagTask marks a tag of a task.
func (t *DBTracer) TagTask(tt TaskTag) {
	originalTask, ok := t.tracingTasks[tt.TaskID]
	if !ok {
		return
	}

	originalTask.Tags = append(originalTask.Tags, tag{
		What:   tt.What,
		Detail: tt.Detail,
	})

	t.tracingTasks[tt.TaskID] = originalTask
}

// EndTask marks the end of a task.
func (t *DBTracer) EndTask(taskEnd TaskEnd) {
	now := t.timeTeller.Now()

	if t.startTime > 0 && now < t.startTime {
		delete(t.tracingTasks, taskEnd.ID)
		return
	}

	originalTask, ok := t.tracingTasks[taskEnd.ID]
	if !ok {
		return
	}

	originalTask.EndTime = now

	delete(t.tracingTasks, taskEnd.ID)

	t.backend.Write(originalTask)
}

// Terminate terminates the tracer.
func (t *DBTracer) Terminate() {
	for _, task := range t.tracingTasks {
		task.EndTime = t.timeTeller.Now()
		t.backend.Write(task)
	}

	t.tracingTasks = nil

	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller TimeTeller,
	backend TracerBackend,
) *DBTracer {
	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      backend,
		tracingTasks: make(map[string]task),
	}

	atexit.Register(func() { t.Terminate() })

	return t
}

// SetTimeRange sets the time range of the tracer.
func (t *DBTracer) SetTimeRange(startTime, endTime float64) {
	t.startTime = startTime
	t.endTime = endTime
}
