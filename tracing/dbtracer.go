package tracing

import (
	"sync"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type taskTableEntry struct {
	ID        string  `json:"id" akita_data:"unique"`
	ParentID  string  `json:"parent_id" akita_data:"index"`
	Kind      string  `json:"kind" akita_data:"index"`
	What      string  `json:"what" akita_data:"index"`
	Location  string  `json:"location" akita_data:"index"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
}

type milestoneTableEntry struct {
	ID       string  `json:"id" akita_data:"unique"`
	TaskID   string  `json:"task_id" akita_data:"index"`
	Time     float64 `json:"time" akita_data:"index"`
	Kind     string  `json:"kind" akita_data:"index"`
	What     string  `json:"what" akita_data:"index"`
	Location string  `json:"location" akita_data:"index"`
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.)
type DBTracer struct {
	mu         sync.Mutex
	timeTeller sim.TimeTeller
	backend    datarecording.DataRecorder

	startTime, endTime sim.VTimeInSec

	tracingTasks map[string]Task
}

// StartTask marks the start of a task.
func (t *DBTracer) StartTask(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.startingTaskMustBeValid(task)

	task.StartTime = t.timeTeller.CurrentTime()
	if t.endTime > 0 && task.StartTime > t.endTime {
		return
	}

	existingTask, found := t.tracingTasks[task.ID]
	if !found {
		t.tracingTasks[task.ID] = task

		return
	}

	existingTask.ParentID = task.ParentID
	existingTask.Kind = task.Kind
	existingTask.What = task.What
	existingTask.Location = task.Location
	existingTask.StartTime = task.StartTime

	t.tracingTasks[task.ID] = existingTask
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
	t.mu.Lock()
	defer t.mu.Unlock()

	milestone.Time = t.timeTeller.CurrentTime()

	task, found := t.tracingTasks[milestone.TaskID]
	if !found {
		task = Task{
			ID: milestone.TaskID,
		}

		task.Milestones = []Milestone{milestone}
		t.tracingTasks[milestone.TaskID] = task

		return
	}

	for _, existingMilestone := range task.Milestones {
		if sameMilestone(existingMilestone, milestone) {
			return
		}
		// Only record the first milestone if multiple milestones occur at the same time
		if existingMilestone.Time == milestone.Time {
			return
		}
	}

	task.Milestones = append(task.Milestones, milestone)
	t.tracingTasks[milestone.TaskID] = task
}

func sameMilestone(a, b Milestone) bool {
	return a.Kind == b.Kind && a.What == b.What && a.Location == b.Location
}

func (t *DBTracer) insertTaskEntry(task Task) {
	taskEntry := taskTableEntry{
		ID:        task.ID,
		ParentID:  task.ParentID,
		Kind:      task.Kind,
		What:      task.What,
		Location:  task.Location,
		StartTime: float64(task.StartTime),
		EndTime:   float64(task.EndTime),
	}
	t.backend.InsertData("trace", taskEntry)
}

func (t *DBTracer) insertMilestones(task Task) {
	for _, milestone := range task.Milestones {
		milestoneEntry := milestoneTableEntry{
			ID:       milestone.ID,
			TaskID:   milestone.TaskID,
			Time:     float64(milestone.Time),
			Kind:     string(milestone.Kind),
			What:     milestone.What,
			Location: milestone.Location,
		}
		t.backend.InsertData("trace_milestones", milestoneEntry)
	}
}

// EndTask marks the end of a task.
func (t *DBTracer) EndTask(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

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

	t.insertTaskEntry(originalTask)
	t.insertMilestones(originalTask)
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
	dataRecorder.CreateTable("trace", taskTableEntry{})
	dataRecorder.CreateTable("trace_milestones", milestoneTableEntry{})

	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      dataRecorder,
		tracingTasks: make(map[string]Task),
	}

	atexit.Register(func() {
		t.Terminate()
	})

	return t
}

// SetTimeRange sets the time range of the tracer.
func (t *DBTracer) SetTimeRange(startTime, endTime sim.VTimeInSec) {
	t.startTime = startTime
	t.endTime = endTime
}
