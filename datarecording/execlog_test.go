package datarecording_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/stretchr/testify/assert"
)

// setupExeLog creates a new ExecRecorder and cleanup function for testing
func setupExecLog(
	t *testing.T,
) (*datarecording.ExecRecorder, datarecording.DataReader, string, func()) {
	t.Helper()

	path := "test"
	initializeTime := time.Now()
	time := initializeTime.Format("2006_01_02_15_04_05")
	logger := datarecording.NewExecRecorder(path)

	reader := datarecording.NewReader(path + ".sqlite3")

	cleanup := func() {
		os.Remove(path + ".sqlite3")
	}

	return logger, reader, time, cleanup
}

// TestSingleExecution tests the logging of a single execution
func TestExecutionLog(t *testing.T) {
	os.Remove("test.sqlite3")

	logger, reader, initializeTime, cleanup := setupExecLog(t)
	defer cleanup()

	originalCL := os.Args
	defer func() { os.Args = originalCL }()

	timeStart := time.Now()
	logger.Write()
	expectedStart := timeStart.Format("2006-01-02 15:04:05")
	tableName := "akita_exec_log_" + initializeTime

	timeEnd := time.Now()
	logger.Flush()
	expectedEnd := timeEnd.Format("2006-01-02 15:04:05")

	assert.True(t, testArgsLog(tableName, reader), "Command should be logged")
	assert.True(t, testPathLog(tableName, reader), "Path should be logged")
	assert.True(t, testStartTimeLog(tableName, expectedStart, reader), "Start time should be logged")
	assert.True(t, testEndTimeLog(tableName, expectedEnd, reader), "End time should be logged")
}

func testArgsLog(tableName string, reader datarecording.DataReader) bool {
	os.Args = []string{"test_program", "arg1", "arg2"}
	expectedCMD := strings.Join(os.Args, " ")

	reader.MapTable(tableName, datarecording.ExecInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[1].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "Command")
		flag = flag && (cmd.Value == expectedCMD)
	}

	return flag
}

func testPathLog(tableName string, reader datarecording.DataReader) bool {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	expectedPath := filepath.Dir(ex)

	reader.MapTable(tableName, datarecording.ExecInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[2].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "CWD")
		flag = flag && (cmd.Value == expectedPath)
	}

	return flag
}

func testStartTimeLog(tableName string, expectedStart string,
	reader datarecording.DataReader) bool {
	reader.MapTable(tableName, datarecording.ExecInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[0].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "Start Time")
		flag = flag && (cmd.Value == expectedStart)
	}

	return flag
}

func testEndTimeLog(tableName string, expectedEnd string,
	reader datarecording.DataReader) bool {
	reader.MapTable(tableName, datarecording.ExecInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[3].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "End Time")
		flag = flag && (cmd.Value == expectedEnd)
	}

	return flag
}
