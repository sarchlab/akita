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
	"github.com/stretchr/testify/require"
)

func setupExeLog(
	t *testing.T,
) (*datarecording.ExeRecorder, *datarecording.DataRecorder,
	*datarecording.DataReader, func()) {
	t.Helper()

	path := "test"
	writer := datarecording.NewDataRecorder(path)

	logger := datarecording.NewExeRecoerder(path)
	logger.SetWriter(writer)

	cleanup := func() {
		os.Remove(path + ".sqlite3")
	}

	return logger, writer, reader, cleanup
}

func TestWrite(t *testing.T) {
	logger, writer, reader, cleanup := setupExeLog(t)
	defer cleanup()

	originalCL := os.Args
	defer func() { os.Args = originalCL }()

	os.Args = []string{"test_program", "arg1", "arg2"}
	logger.Write()

	timeNow := time.Now()
	expectedName := "log_" + timeNow.Format("2006-01-02 15:04:05")
	query := "SELECT name FROM sqlite_master " +
		fmt.Sprintf("WHERE type='table' AND name='%s';", expectedName)

	var tableName string
	err := reader.QueryRow(query).Scan(&tableName)
	require.NoError(t, err, "Table should be created")
	assert.Equal(t, expectedName, tableName, "Table name should match")
}

func TestSingleExecution(t *testing.T) {
	logger, writer, _, cleanup := setupExeLog(t)
	defer cleanup()

	originalCL := os.Args
	defer func() { os.Args = originalCL }()

	os.Args = []string{"test_program", "arg1", "arg2"}
	expectedCMD := strings.Join(os.Args, " ")

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	expectedPath := filepath.Dir(ex)

	timeStart := time.Now()
	logger.Write()
	expectedStart := timeStart.Format("2006-01-02 15:04:05")
	expectedName := "log_" + expectedStart
	query := "SELECT name FROM sqlite_master " +
		fmt.Sprintf("WHERE type='table' AND name='%s';", expectedStart)

	timeEnd := time.Now()
	logger.Flush()
	expectedEnd := timeEnd.Format("2006-01-02 15:04:05")

	var tableName string
	openErr := writer.QueryRow(query).Scan(&tableName)
	require.NoError(t, openErr, "Table should be created")
	assert.Equal(t, expectedName, tableName, "Table name should match")

	var (
		property [4]string
		value    [4]string
	)

	queryCMD := fmt.Sprintf("SELECT Property, Value FROM %s WHERE ID=1;", expectedName)
	errCMD := writer.QueryRow(queryCMD).Scan(&property[0], &value[0])
	require.NoError(t, errCMD, "Data should be inserted")
	assert.Equal(t, "Command", property[0], "Property should match")
	assert.Equal(t, expectedCMD, value[0], "Value should match")

	queryCWD := fmt.Sprintf("SELECT Property, Value FROM %s WHERE ID=2;", expectedName)
	errCWD := writer.QueryRow(queryCWD).Scan(&property[1], &value[1])
	require.NoError(t, errCWD, "Data should be inserted")
	assert.Equal(t, "CWD", property[1], "Property should match")
	assert.Equal(t, expectedPath, value[1], "Value should match")

	queryStart := fmt.Sprintf("SELECT Property, Value FROM %s WHERE ID=3;", expectedName)
	errStart := writer.QueryRow(queryStart).Scan(&property[2], &value[2])
	require.NoError(t, errStart, "Data should be inserted")
	assert.Equal(t, "Start Time", property[2], "Property should match")
	assert.Equal(t, expectedStart, value[2], "Value should match")

	queryEnd := fmt.Sprintf("SELECT Property, Value FROM %s WHERE ID=4;", expectedName)
	errEnd := writer.QueryRow(queryEnd).Scan(&property[3], &value[3])
	require.NoError(t, errEnd, "Data should be inserted")
	assert.Equal(t, "End Time", property[3], "Property should match")
	assert.Equal(t, expectedEnd, value[3], "Value should match")
}
