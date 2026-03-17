// Package trace provides a tracer that can trace memory system tasks.
package trace

import (
	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// memoryTransactionEntry represents a memory transaction in the database
type memoryTransactionEntry struct {
	ID        uint64  `json:"id" akita_data:"unique"`
	Location  string  `json:"location" akita_data:"index"`
	What      string  `json:"what" akita_data:"index"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
	Address   uint64  `json:"address" akita_data:"index"`
	ByteSize  uint64  `json:"byte_size" akita_data:"index"`
}

// memoryStepEntry represents a memory transaction step in the database
type memoryStepEntry struct {
	ID     uint64  `json:"id" akita_data:"unique"`
	TaskID uint64  `json:"task_id" akita_data:"index"`
	Time   float64 `json:"time" akita_data:"index"`
	What   string  `json:"what" akita_data:"index"`
}

// A dbTracer is a hook that can record the actions of a memory model into
// a database using the data recorder.
type dbTracer struct {
	timeTeller         sim.TimeTeller
	dataRecorder       datarecording.DataRecorder
	pendingTransactions map[uint64]*memoryTransactionEntry
}

// NewDBTracer creates a new database-based Tracer.
func NewDBTracer(dataRecorder datarecording.DataRecorder, timeTeller sim.TimeTeller) tracing.Tracer {
	t := &dbTracer{
		timeTeller:          timeTeller,
		dataRecorder:        dataRecorder,
		pendingTransactions: make(map[uint64]*memoryTransactionEntry),
	}

	// Create tables for memory transactions and steps
	t.dataRecorder.CreateTable("memory_transactions", memoryTransactionEntry{})
	t.dataRecorder.CreateTable("memory_steps", memoryStepEntry{})

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

// StepTask marks the memory transaction has completed a milestone
func (t *dbTracer) StepTask(task tracing.Task) {
	if task.Steps == nil || len(task.Steps) == 0 {
		return
	}

	task.Steps[0].Time = t.timeTeller.CurrentTime()

	entry := memoryStepEntry{
		ID:     sim.GetIDGenerator().Generate(),
		TaskID: task.ID,
		Time:   float64(task.Steps[0].Time),
		What:   task.Steps[0].What,
	}

	t.dataRecorder.InsertData("memory_steps", entry)
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
