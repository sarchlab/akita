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
	time := initializeTime.Format("2006-01-02 15:04:05")
	logger := datarecording.NewExecRecoerder(path)

	reader := datarecording.NewReader(path + ".sqlite3")

	cleanup := func() {
		os.Remove(path + ".sqlite3")
	}

	return logger, reader, time, cleanup
}

// TestSingleExecution tests the logging of a single execution
func TestSingleExecution(t *testing.T) {
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

	queryCMD := datarecording.QueryParams{
		Where: "ID=?",
		Args:  []interface{}{1},
	}

	results, _, _ := reader.Query(tableName, queryCMD)

	flag := true
	flag = flag && (len(results) == 1)

	if cmd, ok := results[0].(datarecording.ExecInfo); ok {
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

	queryPath := datarecording.QueryParams{
		Where: "ID=?",
		Args:  []interface{}{2},
	}

	results, _, _ := reader.Query(tableName, queryPath)

	flag := true
	flag = flag && (len(results) == 1)

	if cmd, ok := results[1].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "CWD")
		flag = flag && (cmd.Value == expectedPath)
	}

	return flag
}

func testStartTimeLog(tableName string, expectedStart string,
	reader datarecording.DataReader) bool {
	queryStart := datarecording.QueryParams{
		Where: "ID=?",
		Args:  []interface{}{3},
	}

	results, _, _ := reader.Query(tableName, queryStart)

	flag := true
	flag = flag && (len(results) == 1)

	if cmd, ok := results[2].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "Start Time")
		flag = flag && (cmd.Value == expectedStart)
	}

	return flag
}

func testEndTimeLog(tableName string, expectedEnd string,
	reader datarecording.DataReader) bool {
	queryEnd := datarecording.QueryParams{
		Where: "ID=?",
		Args:  []interface{}{4},
	}

	results, _, _ := reader.Query(tableName, queryEnd)

	flag := true
	flag = flag && (len(results) == 1)

	if cmd, ok := results[3].(datarecording.ExecInfo); ok {
		flag = flag && (cmd.Property == "End Time")
		flag = flag && (cmd.Value == expectedEnd)
	}

	return flag
}

func TestCleanUp(t *testing.T) {
	os.Remove("test.sqlite3")
}
