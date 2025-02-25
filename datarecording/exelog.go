package datarecording

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Struct ExeInfo is feed to SQLiteWriter
type ExeInfo struct {
	property string
	value    string
}

type ExeRecorder struct {
	// ExeRecorder use SQLiteWriter to log execution
	writer *SQLiteWriter

	// tableName string
	tableName string

	// Stores all the entries as into a slice
	entries []ExeInfo
}

// Write keeps track of current execution and writes it into SQLiteWriter
func (e *ExeRecorder) Write() {
	currentTime := time.Now()
	startTime := currentTime.Format("2006-01-02 15:04:05")
	timeEntry := ExeInfo{"Start Time", startTime}
	e.entries = append(e.entries, timeEntry)

	cmd := strings.Join(os.Args, " ")
	cmdEntry := ExeInfo{"Command", cmd}
	e.entries = append(e.entries, cmdEntry)

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	path := filepath.Dir(ex)
	pathEntry := ExeInfo{"Path", path}
	e.entries = append(e.entries, pathEntry)

	sampleEntry := ExeInfo{}
	e.tableName = "log_" + startTime
	e.writer.CreateTable(e.tableName, sampleEntry)
}

// Flush writes data into SQLite along with program exit time
func (e *ExeRecorder) Flush() {
	for _, entry := range e.entries {
		e.writer.InsertData(e.tableName, entry)
	}

	endTime := time.Now()
	endValue := endTime.Format("2006-01-02 15:04:05")
	timeEntry := ExeInfo{"End Time", endValue}
	e.writer.InsertData(e.tableName, timeEntry)

	e.writer.Flush()
}

// Sets SQLiteWriter
func (e *ExeRecorder) SetWriter(inputWriter *SQLiteWriter) {
	e.writer = inputWriter
}

func NewExeRecoerder(
	path string,
) *ExeRecorder {
	return nil
}

func init() {}
