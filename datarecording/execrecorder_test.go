package datarecording_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/stretchr/testify/assert"
	"github.com/tebeka/atexit"
)

// Struct ExecInfo is feed to DataRecorder
type execInfo struct {
	Property string
	Value    string
}

var ExpectedInfo [4]string

// TestRecorderSetUp creates a new ExecRecorder and cleanup function for testing
func TestRecorderSetUp(t *testing.T) {
	defer handlePanic()

	path := "test"

	originalCL := os.Args
	defer func() { os.Args = originalCL }()
	os.Args = []string{"test_program", "arg1", "arg2"}
	ExpectedInfo[0] = strings.Join(os.Args, " ")

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	expectedPath := filepath.Dir(ex)
	ExpectedInfo[1] = expectedPath

	initializeTime := time.Now()
	startTime := initializeTime.Format("2006-01-02 15:04:05.000000000")
	ExpectedInfo[2] = startTime

	writer := datarecording.NewDataRecorder(path)

	assert.True(t, writer != nil)

	exitTime := time.Now()
	endTime := exitTime.Format("2006-01-02 15:04:05.000000000")
	ExpectedInfo[3] = endTime

	fmt.Println(ExpectedInfo)

	atexit.Exit(0)
}

func handlePanic() {
	if r := recover(); r != nil {
		fmt.Println("Recovered from panic:", r)
	}
}

// TestExecutionRecord tests the record of a single execution
func TestExecutionRecord(t *testing.T) {
	path := "test"
	reader := datarecording.NewReader(path + ".sqlite3")

	tableName := "exec_info"

	reader.MapTable(tableName, execInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})
	fmt.Println(results)

	assert.True(t, testArgsLog(tableName, reader), "Command should be logged")
	assert.True(t, testPathLog(tableName, reader), "Path should be logged")
	assert.True(t, testStartTimeLog(tableName, ExpectedInfo[2], reader),
		"Start time should be logged")
	assert.True(t, testEndTimeLog(tableName, ExpectedInfo[3], reader),
		"End time should be logged")
}

func testArgsLog(tableName string, reader datarecording.DataReader) bool {
	expectedCMD := ExpectedInfo[0]

	reader.MapTable(tableName, execInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[1].(execInfo); ok {
		flag = flag && (cmd.Property == "Command")
		flag = flag && (cmd.Value == expectedCMD)
	}

	return flag
}

func testPathLog(tableName string, reader datarecording.DataReader) bool {
	expectedPath := ExpectedInfo[1]

	reader.MapTable(tableName, execInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[2].(execInfo); ok {
		flag = flag && (cmd.Property == "CWD")
		flag = flag && (cmd.Value == expectedPath)
	}

	return flag
}

func testStartTimeLog(
	tableName string,
	expectedStart string,
	reader datarecording.DataReader,
) bool {
	reader.MapTable(tableName, execInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[0].(execInfo); ok {
		flag = flag && (cmd.Property == "Start Time")
		flag = flag && (cmd.Value == expectedStart)
	}

	return flag
}

func testEndTimeLog(
	tableName string,
	expectedEnd string,
	reader datarecording.DataReader,
) bool {
	reader.MapTable(tableName, execInfo{})
	results, _, _ := reader.Query(tableName, datarecording.QueryParams{})

	flag := true
	flag = flag && (len(results) == 4)

	if cmd, ok := results[3].(execInfo); ok {
		flag = flag && (cmd.Property == "End Time")
		flag = flag && (cmd.Value == expectedEnd)
	}

	return flag
}
