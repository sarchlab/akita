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
	EndTime   float64 `json:"end_time" akita_data:"index"` //task的时间
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

	// Write immediately if tracing is enabled
	if t.isTracingFlag && t.currentTableName != "" {
		t.writeTaskToDB(originalTask)
	}

	// Remove from memory immediately
	delete(t.tracingTasks, task.ID)
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
	dataRecorder.CreateTable("trace", traceIndexEntry{})
	dataRecorder.CreateTable("trace_milestones", Milestone{})

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
