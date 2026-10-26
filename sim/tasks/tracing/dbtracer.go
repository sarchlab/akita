package tracing

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Table names for tracing data. The akita$ prefix keeps them apart from the
// tables an application writes into the same recording.
const (
	traceTableName     = "akita$trace"
	milestoneTableName = "akita$milestone"
	tagTableName       = "akita$tag"
	segmentTableName   = "akita$segments"
)

type runningTask struct {
	Tags       []tasks.Tag
	Milestones []tasks.Milestone
}

// No secondary indexes are declared on the trace/milestone/tag/segment columns
// below. The simulation only *writes* this data — it never queries it — so
// maintaining indexes during the run is pure overhead, and an audit of the
// reader's (Daisen's) query plans showed most of these columns are only ever
// filtered alongside Location (served by the reader's Location-led covering
// indexes) or not at all. The reader builds exactly the few indexes its queries
// use, on demand. Location keeps its tag because it drives string interning into
// the shared akita$location table, not just an index.

// taskTableEntry is the table structure for storing task information.
// All tasks are stored in a single akita$trace table.
type taskTableEntry struct {
	ID        uint64  `json:"id"`
	ParentID  uint64  `json:"parent_id"`
	Kind      string  `json:"kind"`
	What      string  `json:"what"`
	Location  string  `json:"location" akita_data:"location"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// milestoneTableEntry is the table structure for storing milestone information.
// All milestones are stored in a single akita$milestone table. A milestone's
// location is inherited from its owning task (joined via TaskID), so it is not
// stored here.
type milestoneTableEntry struct {
	ID     uint64  `json:"id"`
	TaskID uint64  `json:"task_id"`
	Time   float64 `json:"time"`
	Kind   string  `json:"kind"`
	What   string  `json:"what"`
}

// tagTableEntry is the table structure for storing task tag information. All
// tags are stored in a single akita$tag table. Location is inherited from the
// owning task.
type tagTableEntry struct {
	ID     uint64  `json:"id"`
	TaskID uint64  `json:"task_id"`
	Time   float64 `json:"time"`
	What   string  `json:"what"`
}

// segmentTableEntry is the table structure for storing tracing segment information.
// A segment represents a time period between StartTracing and StopTracing calls.
type segmentTableEntry struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.).
// Starting recording mid-run drops earlier tags/milestones of in-flight tasks.
// Their ends still carry the original parent and full interval. The same applies
// to restored tasks observed by a fresh tracer; no start or history is replayed.
type DBTracer struct {
	mu         sync.Mutex
	timeTeller timing.TimeTeller
	backend    datarecording.DataRecorder

	tracingTasks     map[uint64]*runningTask
	isTracing        bool
	tracingStartTime timing.VTimeInPicoSec

	terminated              bool
	firstTerminateBacktrace string
}

// IsTracing reports whether the tracer is currently tracing.
func (t *DBTracer) IsTracing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.isTracing
}

// TaskStarted marks work that overlaps an active recording window.
func (t *DBTracer) TaskStarted(task tasks.Task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.terminated || !t.isTracing {
		return
	}
	t.tracingTasks[task.ID] = &runningTask{}
}

// TagAdded records a tag on a task.
func (t *DBTracer) TagAdded(tag tasks.Tag) {
	t.mu.Lock()
	defer t.mu.Unlock()

	task, found := t.tracingTasks[tag.TaskID]
	if !found {
		if !t.isTracing || t.terminated {
			return
		}
		task = &runningTask{}
		t.tracingTasks[tag.TaskID] = task
	}

	task.Tags = append(task.Tags, tag)
}

// MilestoneAdded adds a milestone.
func (t *DBTracer) MilestoneAdded(milestone tasks.Milestone) {
	t.mu.Lock()
	defer t.mu.Unlock()

	task, found := t.tracingTasks[milestone.TaskID]
	if !found {
		if !t.isTracing || t.terminated {
			return
		}
		task = &runningTask{}
		t.tracingTasks[milestone.TaskID] = task
	}

	for _, existingMilestone := range task.Milestones {
		// Keep at most one milestone per timestamp, but allow a repeated
		// (kind, what) at a different time. A long-lived task (e.g. a wavefront
		// that spans a whole kernel) recurs through the same blocking reasons
		// many times; deduping on (kind, what) regardless of time would collapse
		// its timeline to the first occurrence of each reason. Short-lived tasks
		// hit each reason once, so they are unaffected.
		if existingMilestone.Time == milestone.Time {
			return
		}
	}

	task.Milestones = append(task.Milestones, milestone)
}

// TaskEnded marks the end of a task.
func (t *DBTracer) TaskEnded(task tasks.Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.terminated {
		return
	}
	originalTask, ok := t.tracingTasks[task.ID]
	delete(t.tracingTasks, task.ID)
	if !ok {
		if !t.isTracing {
			return
		}
		originalTask = &runningTask{}
	}

	// Write task to the trace table
	entry := taskTableEntry{
		ID:        task.ID,
		ParentID:  task.ParentID,
		Kind:      task.Kind,
		What:      task.What,
		Location:  task.Location,
		StartTime: float64(task.StartTime),
		EndTime:   float64(task.EndTime),
	}
	t.backend.InsertData(traceTableName, entry)

	// Write milestones to the milestone table. A milestone's location is
	// the task's location, recovered by readers via the TaskID join.
	for _, m := range originalTask.Milestones {
		milestoneEntry := milestoneTableEntry{
			ID:     m.ID,
			TaskID: m.TaskID,
			Time:   float64(m.Time),
			Kind:   string(m.Kind),
			What:   m.What,
		}
		t.backend.InsertData(milestoneTableName, milestoneEntry)
	}

	// Write tags to the tag table.
	for _, tag := range originalTask.Tags {
		tagEntry := tagTableEntry{
			ID:     tag.ID,
			TaskID: tag.TaskID,
			Time:   float64(tag.Time),
			What:   tag.What,
		}
		t.backend.InsertData(tagTableName, tagEntry)
	}
}

// Terminate terminates the tracer. Terminated traces cannot be used again.
func (t *DBTracer) Terminate() {
	t.mu.Lock()

	// Check for double termination
	currentBacktrace := captureBacktrace()
	if t.terminated {
		fmt.Println("ERROR: DBTracer.Terminate called multiple times!")
		fmt.Println("First termination backtrace:")
		fmt.Println(t.firstTerminateBacktrace)
		fmt.Println("Second termination backtrace:")
		fmt.Println(currentBacktrace)
		t.mu.Unlock()
		return
	}

	t.terminated = true
	t.firstTerminateBacktrace = currentBacktrace
	t.mu.Unlock()

	// If tracing is still going, stop it first.
	if t.IsTracing() {
		t.StopTracing()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracingTasks = nil
	t.backend.Flush()
}

func captureBacktrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// StartTracing manually enables tracing.
// Completed tasks carry full timing/identity even when their start was not recorded.
// Earlier tags and milestones are not replayed or backfilled.
func (t *DBTracer) StartTracing() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.isTracing = true
	t.tracingStartTime = t.timeTeller.CurrentTime()
}

// StopTracing stops tracing and finalizes tasks.
func (t *DBTracer) StopTracing() {
	t.mu.Lock()
	defer t.mu.Unlock()

	endTime := t.timeTeller.CurrentTime()

	// Insert segment entry
	segment := segmentTableEntry{
		StartTime: float64(t.tracingStartTime),
		EndTime:   float64(endTime),
	}
	t.backend.InsertData(segmentTableName, segment)

	t.isTracing = false

	// Flush to ensure all data is written to database immediately
	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller timing.TimeTeller,
	dataRecorder datarecording.DataRecorder,
) *DBTracer {
	dataRecorder.CreateTable(traceTableName, taskTableEntry{})
	dataRecorder.CreateTable(milestoneTableName, milestoneTableEntry{})
	dataRecorder.CreateTable(tagTableName, tagTableEntry{})
	dataRecorder.CreateTable(segmentTableName, segmentTableEntry{})

	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      dataRecorder,
		tracingTasks: make(map[uint64]*runningTask),
	}

	return t
}
