package trace_test

import (
	"fmt"
	"os"
	"sort"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/sarchlab/akita/v4/mem/trace"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// SimpleTimeTeller implements sim.TimeTeller for example
type SimpleTimeTeller struct {
	currentTime sim.VTimeInSec
}

func (t *SimpleTimeTeller) CurrentTime() sim.VTimeInSec {
	return t.currentTime
}

func (t *SimpleTimeTeller) AdvanceTime(duration sim.VTimeInSec) {
	t.currentTime += duration
}

// ExampleReadReq implements mem.AccessReq for example
type ExampleReadReq struct {
	sim.MsgMeta

	address  uint64
	byteSize uint64
	pid      vm.PID
}

func (r *ExampleReadReq) Meta() *sim.MsgMeta {
	return &r.MsgMeta
}

func (r *ExampleReadReq) Clone() sim.Msg {
	return &ExampleReadReq{
		MsgMeta:  r.MsgMeta,
		address:  r.address,
		byteSize: r.byteSize,
		pid:      r.pid,
	}
}

func (r *ExampleReadReq) GetAddress() uint64 {
	return r.address
}

func (r *ExampleReadReq) GetByteSize() uint64 {
	return r.byteSize
}

func (r *ExampleReadReq) GetPID() vm.PID {
	return r.pid
}

// Example demonstrates using the database-based memory tracer
func Example() {
	dbPath := "memory_trace_example"
	os.Remove(dbPath + ".sqlite3")

	// Create a data recorder
	dataRecorder := datarecording.NewDataRecorder(dbPath)

	// Create a time teller
	timeTeller := &SimpleTimeTeller{currentTime: 0}

	// Create the database-based memory tracer
	memTracer := trace.NewDBTracer(dataRecorder, timeTeller)

	runExampleTrace(memTracer, timeTeller)

	// Flush data to database
	dataRecorder.Flush()

	// List available tables
	tables := dataRecorder.ListTables()
	sort.Strings(tables) // Sort tables for consistent output
	fmt.Printf("Tables created: %v\n", tables)

	fmt.Println("Memory trace example completed successfully!")
	fmt.Printf("Database saved to: %s.sqlite3\n", dbPath)

	// Clean up
	dataRecorder.Close()
	os.Remove(dbPath + ".sqlite3")

	// Output:
	// Starting memory trace example...
	// Started memory read at time 100.0 ns
	// Cache miss recorded at time 150.0 ns
	// Completed memory read at time 200.0 ns
	// Tables created: [exec_info memory_steps memory_transactions]
	// Memory trace example completed successfully!
	// Database saved to: memory_trace_example.sqlite3
}

func runExampleTrace(memTracer tracing.Tracer, timeTeller *SimpleTimeTeller) {
	// Simulate a memory read operation
	readReq := &ExampleReadReq{
		address:  0x1000,
		byteSize: 64,
		pid:      1,
	}

	// Create a memory task
	task := tracing.Task{
		ID:       "mem_read_001",
		Location: "L1_cache",
		What:     "read",
		Detail:   readReq,
	}

	// Trace the complete memory operation lifecycle
	fmt.Println("Starting memory trace example...")

	// Start the task at time 100ns
	timeTeller.AdvanceTime(100)
	memTracer.StartTask(task)
	fmt.Printf("Started memory read at time %.1f ns\n", float64(timeTeller.CurrentTime()))

	// Add a step (cache miss)
	timeTeller.AdvanceTime(50)
	task.Steps = []tracing.TaskStep{{What: "cache_miss"}}
	memTracer.StepTask(task)
	fmt.Printf("Cache miss recorded at time %.1f ns\n", float64(timeTeller.CurrentTime()))

	// End the task at time 200ns
	timeTeller.AdvanceTime(50)
	memTracer.EndTask(task)
	fmt.Printf("Completed memory read at time %.1f ns\n", float64(timeTeller.CurrentTime()))
}