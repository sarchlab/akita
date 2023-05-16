package tracing

import (
	"fmt"
	"os"

	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// CSVTraceWriter is a task tracer that can store the tasks into a CSV file.
type CSVTraceWriter struct {
	path string
	file *os.File

	tasks      []Task
	bufferSize int
}

// NewCSVTraceWriter creates a new CSVTracerBackend.
func NewCSVTraceWriter(path string) *CSVTraceWriter {
	return &CSVTraceWriter{
		path:       path,
		bufferSize: 1000,
	}
}

// Init creates the tracing csv file. If the file already exists, it will be
// overwritten.
func (t *CSVTraceWriter) Init() {
	if t.path == "" {
		t.path = "akita_trace_" + xid.New().String()
	}

	filename := t.path + ".csv"
	_, err := os.Stat(filename)
	if err == nil {
		panic(fmt.Errorf("file %s already exists", filename))
	}

	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	t.file = file

	fmt.Fprintf(file, "ID, ParentID, Kind, What, Where, Start, End\n")

	atexit.Register(func() {
		t.Flush()
		err := t.file.Close()
		if err != nil {
			panic(err)
		}
	})
}

// Write writes a task to the CSV file.
func (t *CSVTraceWriter) Write(task Task) {
	t.tasks = append(t.tasks, task)
	if len(t.tasks) >= t.bufferSize {
		t.Flush()
	}
}

// Flush flushes the tasks to the CSV file.
func (t *CSVTraceWriter) Flush() {
	for _, task := range t.tasks {
		fmt.Fprintf(t.file, "%s, %s, %s, %s, %s, %.10f, %.10f\n",
			task.ID,
			task.ParentID,
			task.Kind,
			task.What,
			task.Where,
			task.StartTime,
			task.EndTime,
		)
	}

	t.tasks = nil
}
