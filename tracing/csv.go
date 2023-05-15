package tracing

import (
	"fmt"
	"os"

	"github.com/tebeka/atexit"
)

// CSVTracerBackend is a task tracer that can store the tasks into a CSV file.
type CSVTracerBackend struct {
	path string
	file *os.File

	tasks      []Task
	bufferSize int
}

// NewCSVTracerBackend creates a new CSVTracerBackend.
func NewCSVTracerBackend(path string) *CSVTracerBackend {
	return &CSVTracerBackend{
		path:       path,
		bufferSize: 1000,
	}
}

// Init creates the tracing csv file. If the file already exists, it will be
// overwritten.
func (t *CSVTracerBackend) Init() {
	file, err := os.Create(t.path)
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
func (t *CSVTracerBackend) Write(task Task) {
	t.tasks = append(t.tasks, task)
	if len(t.tasks) >= t.bufferSize {
		t.Flush()
	}
}

// Flush flushes the tasks to the CSV file.
func (t *CSVTracerBackend) Flush() {
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
