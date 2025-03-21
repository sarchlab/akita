package datarecording

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Struct ExecInfo is feed to DataRecorder
type ExecInfo struct {
	property string
	value    string
}

// Records program execution
type ExecRecorder struct {
	db        *sql.DB
	recorder  DataRecorder
	tableName string
	entries   []ExecInfo
}

func (e *ExecRecorder) Init() {
}

// Write keeps track of current execution and writes it into SQLiteWriter
func (e *ExecRecorder) Write() {
	currentTime := time.Now()
	startTime := currentTime.Format("2006-01-02 15:04:05")
	timeEntry := ExecInfo{"Start Time", startTime}
	e.entries = append(e.entries, timeEntry)

	cmd := strings.Join(os.Args, " ")
	cmdEntry := ExecInfo{"Command", cmd}
	e.entries = append(e.entries, cmdEntry)

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	path := filepath.Dir(ex)
	pathEntry := ExecInfo{"Path", path}
	e.entries = append(e.entries, pathEntry)

	sampleEntry := ExecInfo{}
	e.tableName = "log_" + startTime
	e.recorder.CreateTable(e.tableName, sampleEntry)
}

// Flush writes data into SQLite along with program exit time
func (e *ExecRecorder) Flush() {
	for _, entry := range e.entries {
		e.recorder.InsertData(e.tableName, entry)
	}

	endTime := time.Now()
	endValue := endTime.Format("2006-01-02 15:04:05")
	timeEntry := ExecInfo{"End Time", endValue}
	e.recorder.InsertData(e.tableName, timeEntry)

	e.recorder.Flush()
}

func NewExeRecoerder(
	path string,
) *ExecRecorder {
	return nil
}
