package datarecording

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tebeka/atexit"
)

// Struct ExecInfo is feed to DataRecorder
type ExecInfo struct {
	Property string
	Value    string
}

// Records program execution
type ExecRecorder struct {
	// db        *sql.DB
	tablename string
	recorder  DataRecorder
	entries   []ExecInfo
}

// Write log current execution
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
}

// Flush writes data into SQLite along with program exit time
func (e *ExecRecorder) Flush() {
	for _, entry := range e.entries {
		e.recorder.InsertData(e.tablename, entry)
	}

	endTime := time.Now()
	endValue := endTime.Format("2006-01-02 15:04:05")
	timeEntry := ExecInfo{"End Time", endValue}
	e.recorder.InsertData(e.tablename, timeEntry)

	e.recorder.Flush()
}

// NewExecRecorder creates a new ExecRecorder
func NewExecRecoerder(path string) *ExecRecorder {
	newRecorder := NewDataRecorder(path)

	e := &ExecRecorder{
		recorder: newRecorder,
	}
	setupTable(e)

	atexit.Register(e.Flush)

	return e
}

// NewExecRecorderWithDB creates a new ExecRecorder with a given database
func NewExecRecorderWithDB(db *sql.DB) *ExecRecorder {
	newRecorder := NewDataRecorderWithDB(db)

	e := &ExecRecorder{
		recorder: newRecorder,
	}
	setupTable(e)

	atexit.Register(e.Flush)

	return e
}

func setupTable(e *ExecRecorder) {
	currentTime := time.Now()
	time := currentTime.Format("2006:01:02 15:04:05")
	name := "akita_exec_log_" + time
	e.tablename = name

	sampleEntry := ExecInfo{}
	e.recorder.CreateTable(name, sampleEntry)
}
