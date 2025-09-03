package tracing

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type taskTableEntry struct {
	ID        string  `json:"id" akita_data:"index"`
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

// traceIndexEntry 是trace表的索引结构，只包含session信息
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

	startTime, endTime sim.VTimeInSec

	tracingTasks  map[string]Task
	isTracingFlag bool // Optional: internal flag for manual control

	traceCount       int
	currentTableName string
	sessionStartTime sim.VTimeInSec
	sessionEndTime   sim.VTimeInSec

	batchBuffer []taskTableEntry // 预分配缓冲区
}

// to check later
func (t *DBTracer) IsTracing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isTracingFlag
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

	// 将任务写入对应的task表
	taskTable := taskTableEntry{
		ID:        originalTask.ID,
		ParentID:  originalTask.ParentID,
		Kind:      originalTask.Kind,
		What:      originalTask.What,
		Location:  originalTask.Location,
		StartTime: float64(originalTask.StartTime),
		EndTime:   float64(originalTask.EndTime),
	}
	t.backend.InsertData(t.currentTableName, taskTable) //写入trace_i表(其实可以不用上面的function)
	t.insertMilestones(originalTask)
}

// Terminate terminates the tracer.
func (t *DBTracer) Terminate() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// do not write anything

	t.tracingTasks = nil
	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller sim.TimeTeller,
	dataRecorder datarecording.DataRecorder,
) *DBTracer {
	dataRecorder.CreateTable("trace", traceIndexEntry{}) // 使用索引结构
	dataRecorder.CreateTable("trace_milestones", milestoneTableEntry{})
	//dataRecorder.CreateTable("trace", taskTableEntry{}) 最新版

	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      dataRecorder,
		tracingTasks: make(map[string]Task),
		batchBuffer:  make([]taskTableEntry, 0, 1000),
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
	print("Enable tracing at current time...\n")
	t.mu.Lock()
	defer t.mu.Unlock()

	// 清除之前的内存
	t.tracingTasks = make(map[string]Task)

	t.isTracingFlag = true
	t.traceCount++
	t.sessionStartTime = t.timeTeller.CurrentTime()
	t.sessionEndTime = 0
	t.currentTableName = fmt.Sprintf("trace%d", t.traceCount)
	t.backend.CreateTable(t.currentTableName, taskTableEntry{})
}

// StopTracingAtCurrentTime stops tracing and finalizes tasks.
func (t *DBTracer) StopTracingAtCurrentTime() {
	print("Stopping tracing at current time...\n")
	t.mu.Lock()
	defer t.mu.Unlock()

	t.isTracingFlag = false
	t.sessionEndTime = t.timeTeller.CurrentTime()

	// Write session index
	traceIndex := traceIndexEntry{
		TableName:    t.currentTableName,
		SessionStart: float64(t.sessionStartTime),
		SessionEnd:   float64(t.sessionEndTime),
	}
	t.backend.InsertData("trace", traceIndex)

	// Write ongoing tasks
	t.batchWriteOngoingTasks()

	// Clear memory
	t.tracingTasks = make(map[string]Task)
	t.backend.Flush()
}

// 提取公共方法
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

	// 直接写入，不需要额外的缓冲区
	for _, task := range t.tracingTasks {
		if task.StartTime <= t.sessionEndTime {
			// 创建临时任务，设置结束时间
			tempTask := task
			tempTask.EndTime = t.sessionEndTime
			t.writeTaskToDB(tempTask)
		}
	}
}
