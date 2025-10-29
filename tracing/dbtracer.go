package tracing

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type runningTask struct {
	Task

	toRecord bool
}

// taskTableEntry is the table structure containing task information
// multiple tasktable can be created along with click events for tracing
// (trace1, trace2, ...)
type taskTableEntry struct {
	ID        string  `json:"id" akita_data:"unique"`
	ParentID  string  `json:"parent_id" akita_data:"index"`
	Kind      string  `json:"kind" akita_data:"index"`
	What      string  `json:"what" akita_data:"index"`
	Location  string  `json:"location" akita_data:"index"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
}

// milestoneTableEntry is the table structure containing milestone information
// multiple milestonetable can be created along with click events for tracing
// (trace1_milestone, trace2_milestone, ...)
type milestoneTableEntry struct {
	ID       string  `json:"id" akita_data:"unique"` // unique
	TaskID   string  `json:"task_id" akita_data:"index"`
	Time     float64 `json:"time" akita_data:"index"`
	Kind     string  `json:"kind" akita_data:"index"`
	What     string  `json:"what" akita_data:"index"`
	Location string  `json:"location" akita_data:"index"`
}

// traceIndexEntry is the index structure for tracingsession information
// only one traceIndexEntry will be created to store all tracing session time.
// (trace)
type traceIndexEntry struct {
	TableName    string  `json:"table_name" akita_data:"unique"`
	SessionStart float64 `json:"session_start" akita_data:"index"`
	SessionEnd   float64 `json:"session_end" akita_data:"index"`
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.)
type DBTracer struct {
	mu         sync.Mutex
	timeTeller sim.TimeTeller
	backend    datarecording.DataRecorder

	tracingTasks     map[string]runningTask
	isTracing        bool
	tracingStartTime sim.VTimeInSec
}

// IsTracing reports whether the tracer is currently tracing.
func (t *DBTracer) IsTracing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.isTracing
}

// StartTask marks the start of a task.
func (t *DBTracer) StartTask(task Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.startingTaskMustBeValid(task)

	// A task may first be mentioned by a milestone
	rt, found := t.tracingTasks[task.ID]
	if !found {
		rt = runningTask{
			Task: task,
		}
	}

	rt.ParentID = task.ParentID
	rt.Kind = task.Kind
	rt.What = task.What
	rt.Location = task.Location
	rt.StartTime = t.timeTeller.CurrentTime()

	if t.isTracing {
		rt.toRecord = true
	}

	t.tracingTasks[task.ID] = rt
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
		task = runningTask{}
		task.ID = milestone.TaskID

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

	// Only write data when tracing is enabled
	if !t.isTracingFlag || t.currentTableName == "" {
		return
	}

	// Write task to the corresponding task table
	taskTable := taskTableEntry{
		ID:        originalTask.ID,
		ParentID:  originalTask.ParentID,
		Kind:      originalTask.Kind,
		What:      originalTask.What,
		Location:  originalTask.Location,
		StartTime: float64(originalTask.StartTime),
		EndTime:   float64(originalTask.EndTime),
	}
	t.backend.InsertData(t.currentTableName, taskTable) // Write to trace_i table
	//t.insertMilestones(originalTask)
}

// Terminate terminates the tracer.
func (t *DBTracer) Terminate() {
	// If tracing is still enabled, stop it first to write the final session index
	if t.IsTracing() {
		t.StopTracingAtCurrentTime()
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracingTasks = nil
	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller sim.TimeTeller,
	dataRecorder datarecording.DataRecorder,
) *DBTracer {
	dataRecorder.CreateTable("trace", traceIndexEntry{}) // Use index structure

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

// EnableTracing manually enables tracing.
func (t *DBTracer) EnableTracing() {
	print("DBTracer: Enable tracing at current time...\n")
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear previous memory
	t.tracingTasks = make(map[string]Task)

	t.isTracingFlag = true
	t.traceCount++
	t.sessionStartTime = t.timeTeller.CurrentTime()
	t.sessionEndTime = 0
	t.currentTableName = fmt.Sprintf("trace%d", t.traceCount)
	t.backend.CreateTable(t.currentTableName, taskTableEntry{})

	// Create corresponding milestone table (e.g., milestone_trace1)
	milestoneTableName := fmt.Sprintf("%s_milestone", t.currentTableName)
	t.backend.CreateTable(milestoneTableName, milestoneTableEntry{})
}

// StopTracingAtCurrentTime stops tracing and finalizes tasks.
func (t *DBTracer) StopTracingAtCurrentTime() {
	print("DBTracer: Stopping tracing at current time...\n")
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sessionEndTime = t.timeTeller.CurrentTime()

	// Write session index
	traceIndex := traceIndexEntry{
		TableName:    t.currentTableName,
		SessionStart: float64(t.sessionStartTime),
		SessionEnd:   float64(t.sessionEndTime),
	}
	t.backend.InsertData("trace", traceIndex)

	// Write ongoing tasks (must be done before setting isTracingFlag=false)
	t.batchWriteOngoingTasks()

	// Write milestones within session time range to milestone table
	t.writeMilestonesInSession()

	// Set flag to false only at the end to ensure data is written
	t.isTracingFlag = false

	// Clear memory
	t.tracingTasks = make(map[string]Task)

	// Flush to ensure all data is written to database immediately
	t.backend.Flush()
}

// writeMilestonesInSession writes milestones within session time range to milestone table
func (t *DBTracer) writeMilestonesInSession() {
	milestoneTableName := fmt.Sprintf("%s_milestone", t.currentTableName)

	for _, task := range t.tracingTasks {
		for _, milestone := range task.Milestones {
			// Only write milestones within session time range
			if milestone.Time >= t.sessionStartTime && milestone.Time <= t.sessionEndTime {
				milestoneEntry := milestoneTableEntry{
					ID:       milestone.ID,
					TaskID:   milestone.TaskID,
					Time:     float64(milestone.Time),
					Kind:     string(milestone.Kind),
					What:     milestone.What,
					Location: milestone.Location,
				}
				t.backend.InsertData(milestoneTableName, milestoneEntry)
			}
		}
	}
}

// writeTaskToDB is a common method to write a task to the database
func (t *DBTracer) writeTaskToDB(task Task) {
	taskTable := taskTableEntry{
		ID:        task.ID,
		ParentID:  task.ParentID,
		Kind:      task.Kind,
		What:      task.What,
		Location:  task.Location,
		StartTime: float64(task.StartTime),
		EndTime:   float64(task.EndTime),
	}
	t.backend.InsertData(t.currentTableName, taskTable)
}

// batchWriteOngoingTasks efficiently writes all ongoing tasks in batches
func (t *DBTracer) batchWriteOngoingTasks() {
	if !t.isTracingFlag || t.currentTableName == "" {
		return
	}

	// Write directly, no additional buffer needed
	for _, task := range t.tracingTasks {
		if task.StartTime <= t.sessionEndTime {
			// Create temporary task and set end time
			tempTask := task
			tempTask.EndTime = t.sessionEndTime
			t.writeTaskToDB(tempTask)
		}
	}
}
