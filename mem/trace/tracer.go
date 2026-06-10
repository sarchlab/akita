// Package trace provides a tracer that can trace memory system tasks.
package trace

import (
	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/mem/memprotocol"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// memoryTransactionEntry represents a memory transaction in the database
type memoryTransactionEntry struct {
	ID        uint64  `json:"id"         akita_data:"unique"`
	Location  string  `json:"location"   akita_data:"index"`
	What      string  `json:"what"       akita_data:"index"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time"   akita_data:"index"`
	Address   uint64  `json:"address"    akita_data:"index"`
	ByteSize  uint64  `json:"byte_size"  akita_data:"index"`
}

// memoryTagEntry represents a memory transaction tag in the database
type memoryTagEntry struct {
	ID     uint64  `json:"id"      akita_data:"unique"`
	TaskID uint64  `json:"task_id" akita_data:"index"`
	Time   float64 `json:"time"    akita_data:"index"`
	What   string  `json:"what"    akita_data:"index"`
}

// A dbTracer is a hook that can record the actions of a memory model into
// a database using the data recorder.
type dbTracer struct {
	tracing.NopTracer

	dataRecorder        datarecording.DataRecorder
	pendingTransactions map[uint64]*memoryTransactionEntry
}

// NewDBTracer creates a new database-based Tracer.
func NewDBTracer(dataRecorder datarecording.DataRecorder) tracing.Tracer {
	t := &dbTracer{
		dataRecorder:        dataRecorder,
		pendingTransactions: make(map[uint64]*memoryTransactionEntry),
	}

	// Create tables for memory transactions and tags
	t.dataRecorder.CreateTable("memory_transactions", memoryTransactionEntry{})
	t.dataRecorder.CreateTable("memory_tags", memoryTagEntry{})

	return t
}

// StartTask marks the start of a memory transaction
func (t *dbTracer) StartTask(task tracing.TaskStart) {
	req, ok := task.Detail.(memprotocol.AccessReq)
	if !ok {
		return
	}

	entry := &memoryTransactionEntry{
		ID:        task.ID,
		Location:  task.Location,
		What:      task.What,
		StartTime: float64(task.Time),
		EndTime:   0, // Will be set in EndTask
		Address:   req.GetAddress(),
		ByteSize:  req.GetByteSize(),
	}

	t.pendingTransactions[task.ID] = entry
}

// AddTaskTag records a tag on a memory transaction
func (t *dbTracer) AddTaskTag(tag tracing.TaskTag) {
	entry := memoryTagEntry{
		ID:     timing.GetIDGenerator().Generate(),
		TaskID: tag.TaskID,
		Time:   float64(tag.Time),
		What:   tag.What,
	}

	t.dataRecorder.InsertData("memory_tags", entry)
}

// EndTask marks the end of a memory transaction
func (t *dbTracer) EndTask(task tracing.TaskEnd) {
	entry, exists := t.pendingTransactions[task.ID]
	if !exists {
		return
	}

	entry.EndTime = float64(task.Time)
	t.dataRecorder.InsertData("memory_transactions", *entry)

	delete(t.pendingTransactions, task.ID)
}
