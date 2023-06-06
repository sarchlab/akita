package tracing

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v3/sim"

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

// CSVTraceReader is a task tracer that can read tasks from a CSV file.
type CSVTraceReader struct {
	path string
}

// NewCSVTraceReader creates a new CSVTraceReader.
func NewCSVTraceReader(path string) *CSVTraceReader {
	r := &CSVTraceReader{
		path: path,
	}

	return r
}

// ListComponents queries the components that have tasks.
func (r *CSVTraceReader) ListComponents() []string {
	f, err := os.Open(r.path)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}()

	reader := csv.NewReader(f)
	r.skipCSVHeader(reader)

	components := make(map[string]bool)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		task := r.parseCSVRecord(record)

		components[task.Where] = true
	}

	componentSlice := make([]string, 0, len(components))
	for k := range components {
		componentSlice = append(componentSlice, k)
	}

	sort.Strings(componentSlice)

	return componentSlice
}

// ListTasks queries tasks .
func (r *CSVTraceReader) ListTasks(query TaskQuery) []Task {
	f, err := os.Open(r.path)
	if err != nil {
		panic(err)
	}

	defer func() {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}()

	reader := csv.NewReader(f)
	r.skipCSVHeader(reader)

	tasks := make([]Task, 0)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			panic(err)
		}

		task := r.parseCSVRecord(record)

		if !r.keepTask(task, query) {
			continue
		}

		if query.EnableParentTask {
			parentTasks := r.ListTasks(TaskQuery{
				ID: task.ParentID,
			})

			if len(parentTasks) != 0 {
				task.ParentTask = &parentTasks[0]
			}
		}

		tasks = append(tasks, task)
	}

	return tasks
}

func (r *CSVTraceReader) keepTask(task Task, query TaskQuery) bool {
	if !r.meetIDRequirement(task, query) {
		return false
	}

	if !r.meetParentIDRequirement(task, query) {
		return false
	}

	if !r.meetKindRequirement(task, query) {
		return false
	}

	if !r.meetWhereRequirement(task, query) {
		return false
	}

	if !r.meetTimeRangeRequirement(task, query) {
		return false
	}

	return true
}

func (*CSVTraceReader) meetIDRequirement(task Task, query TaskQuery) bool {
	if query.ID != "" && task.ID != query.ID {
		return false
	}

	return true
}

func (*CSVTraceReader) meetParentIDRequirement(
	task Task,
	query TaskQuery,
) bool {
	if query.ParentID != "" && task.ParentID != query.ParentID {
		return false
	}

	return true
}

func (*CSVTraceReader) meetKindRequirement(task Task, query TaskQuery) bool {
	if query.Kind != "" && task.Kind != query.Kind {
		return false
	}

	return true
}

func (*CSVTraceReader) meetWhereRequirement(task Task, query TaskQuery) bool {
	if query.Where != "" && task.Where != query.Where {
		return false
	}

	return true
}

func (*CSVTraceReader) meetTimeRangeRequirement(
	task Task,
	query TaskQuery,
) bool {
	if query.EnableTimeRange {
		if float64(task.EndTime) < query.StartTime ||
			float64(task.StartTime) > query.EndTime {
			return false
		}
	}

	return true
}

func (*CSVTraceReader) skipCSVHeader(r *csv.Reader) {
	_, err := r.Read()
	if err != nil {
		panic(err)
	}
}

func (*CSVTraceReader) parseCSVRecord(record []string) Task {
	task := Task{}
	task.ID = strings.Trim(record[0], " ")
	task.ParentID = strings.Trim(record[1], " ")
	task.Kind = strings.Trim(record[2], " ")
	task.What = strings.Trim(record[3], " ")
	task.Where = strings.Trim(record[4], " ")

	startTime, err := strconv.ParseFloat(strings.Trim(record[5], " "), 64)
	if err != nil {
		panic(err)
	}

	task.StartTime = sim.VTimeInSec(startTime)

	endTime, err := strconv.ParseFloat(strings.Trim(record[6], " "), 64)
	if err != nil {
		panic(err)
	}

	task.EndTime = sim.VTimeInSec(endTime)
	return task
}
