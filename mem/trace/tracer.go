// Package trace provides a tracer that can trace memory system tasks.
package trace

import (
	"log"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/instrumentation/tracing"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// memoryTransactionEntry represents a memory transaction in the database
type memoryTransactionEntry struct {
	ID        string  `json:"id" akita_data:"unique"`
	Location  string  `json:"location" akita_data:"index"`
	What      string  `json:"what" akita_data:"index"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
	Address   uint64  `json:"address" akita_data:"index"`
	ByteSize  uint64  `json:"byte_size" akita_data:"index"`
}

// memoryTagEntry represents a memory transaction tag in the database
type memoryTagEntry struct {
	ID     string  `json:"id" akita_data:"unique"`
	TaskID string  `json:"task_id" akita_data:"index"`
	Time   float64 `json:"time" akita_data:"index"`
	What   string  `json:"what" akita_data:"index"`
}

// A tracer is a hook that can record the actions of a memory model into
// traces.
type tracer struct {
	timeTeller sim.TimeTeller
	logger     *log.Logger
}

// A dbTracer is a hook that can record the actions of a memory model into
// a database using the data recorder.
type dbTracer struct {
	timeTeller          sim.TimeTeller
	dataRecorder        datarecording.DataRecorder
	pendingTransactions map[string]*memoryTransactionEntry
}

// StartTask marks the start of a memory transaction
func (t *tracer) StartTask(task tracing.Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	req, ok := task.Detail.(mem.AccessReq)
	if !ok {
		return
	}

	t.logger.Printf(
		"start, %.12f, %s, %s, %s, 0x%x, %d\n",
		task.StartTime,
		task.Location,
		task.ID,
		task.What,
		req.GetAddress(),
		req.GetByteSize(),
	)
}

// TagTask marks that the memory transaction has been tagged
func (t *tracer) TagTask(task tracing.Task) {
	task.Tags[0].Time = t.timeTeller.CurrentTime()

	t.logger.Printf("tag, %.12f, %s, %s\n",
		task.Tags[0].Time,
		task.ID,
		task.Tags[0].What)
}

// AddMilestone adds a milestone to the task
func (t *tracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing
}

// EndTask marks the end of a memory transaction
func (t *tracer) EndTask(task tracing.Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	t.logger.Printf("end, %.12f, %s\n", task.EndTime, task.ID)
}

// NewTracer creates a new Tracer.
// Deprecated: Use NewDBTracer instead for structured database storage.
func NewTracer(logger *log.Logger, timeTeller sim.TimeTeller) tracing.Tracer {
	t := new(tracer)
	t.logger = logger
	t.timeTeller = timeTeller

	return t
}

// NewDBTracer creates a new database-based Tracer.
func NewDBTracer(dataRecorder datarecording.DataRecorder, timeTeller sim.TimeTeller) tracing.Tracer {
	t := &dbTracer{
		timeTeller:          timeTeller,
		dataRecorder:        dataRecorder,
		pendingTransactions: make(map[string]*memoryTransactionEntry),
	}

	// Create tables for memory transactions and tags
	t.dataRecorder.CreateTable("memory_transactions", memoryTransactionEntry{})
	t.dataRecorder.CreateTable("memory_tags", memoryTagEntry{})

	return t
}

// StartTask marks the start of a memory transaction
func (t *dbTracer) StartTask(task tracing.Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	req, ok := task.Detail.(mem.AccessReq)
	if !ok {
		return
	}

	entry := &memoryTransactionEntry{
		ID:        task.ID,
		Location:  task.Location,
		What:      task.What,
		StartTime: float64(task.StartTime),
		EndTime:   0, // Will be set in EndTask
		Address:   req.GetAddress(),
		ByteSize:  req.GetByteSize(),
	}

	t.pendingTransactions[task.ID] = entry
}

// TagTask marks that the memory transaction has been tagged
func (t *dbTracer) TagTask(task tracing.Task) {
	if task.Tags == nil || len(task.Tags) == 0 {
		return
	}

	task.Tags[0].Time = t.timeTeller.CurrentTime()

	entry := memoryTagEntry{
		ID:     task.ID + "_tag_" + task.Tags[0].What,
		TaskID: task.ID,
		Time:   float64(task.Tags[0].Time),
		What:   task.Tags[0].What,
	}

	t.dataRecorder.InsertData("memory_tags", entry)
}

// AddMilestone adds a milestone to the task
func (t *dbTracer) AddMilestone(milestone tracing.Milestone) {
	// Do nothing - not relevant for memory tracing
}

// EndTask marks the end of a memory transaction
func (t *dbTracer) EndTask(task tracing.Task) {
	// Get the pending transaction and complete it
	entry, exists := t.pendingTransactions[task.ID]
	if !exists {
		return
	}

	if task.EndTime == 0 {
		task.EndTime = t.timeTeller.CurrentTime()
	}

	entry.EndTime = float64(task.EndTime)
	t.dataRecorder.InsertData("memory_transactions", *entry)

	// Remove from pending transactions
	delete(t.pendingTransactions, task.ID)
}
