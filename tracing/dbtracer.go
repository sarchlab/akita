package tracing

import (
	"fmt"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type taskTableEntry struct {
	ID        string  `json:"id"`
	ParentID  string  `json:"parent_id"`
	Kind      string  `json:"kind"`
	What      string  `json:"what"`
	Location  string  `json:"location"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.)
type DBTracer struct {
	timeTeller sim.TimeTeller
	backend    datarecording.DataRecorder

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

	if task.Location == "" {
		panic("task location must be set")
	}
}

// StepTask marks a step of a task.
func (t *DBTracer) StepTask(_ Task) {
	// Do nothing for now.
}

// AddMilestone adds a milestone.
func (t *DBTracer) AddMilestone(milestone Milestone) {
	t.backend.InsertData("trace_milestones", milestone)
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

	taskTable := taskTableEntry{
		ID:        originalTask.ID,
		ParentID:  originalTask.ParentID,
		Kind:      originalTask.Kind,
		What:      originalTask.What,
		Location:  originalTask.Location,
		StartTime: float64(originalTask.StartTime),
		EndTime:   float64(originalTask.EndTime),
	}

	t.backend.InsertData("trace", taskTable)
}

// Terminate terminates the tracer.
func (t *DBTracer) Terminate() {
	for _, task := range t.tracingTasks {
		task.EndTime = t.timeTeller.CurrentTime()
		taskTable := taskTableEntry{
			ID:        task.ID,
			ParentID:  task.ParentID,
			Kind:      task.Kind,
			What:      task.What,
			Location:  task.Location,
			StartTime: float64(task.StartTime),
			EndTime:   float64(task.EndTime),
		}
		t.backend.InsertData("trace", taskTable)
	}

	t.tracingTasks = nil

	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller sim.TimeTeller,
	dataRecorder datarecording.DataRecorder,
) *DBTracer {
	fmt.Println("Creating 'trace' table")
	dataRecorder.CreateTable("trace", taskTableEntry{})
	dataRecorder.Flush()

	fmt.Println("Creating 'trace_milestones' table")
	dataRecorder.CreateTable("trace_milestones", Milestone{})
	dataRecorder.Flush()
	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      dataRecorder,
		tracingTasks: make(map[string]Task),
	}

	atexit.Register(func() {
		t.Terminate()
		t.backend.Flush()
	})
	return t
}

// SetTimeRange sets the time range of the tracer.
func (t *DBTracer) SetTimeRange(startTime, endTime sim.VTimeInSec) {
	t.startTime = startTime
	t.endTime = endTime
}
