package datarecording_test

import (
	"context"
	"os"
	"testing"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/stretchr/testify/assert"
)

// Struct ExecInfo is feed to DataRecorder
type execInfo struct {
	Property string
	Value    string
}

// TestDataRecorderExecution tests that the data recorder properly records
// execution information
func TestDataRecorderExecution(t *testing.T) {
	path := "test"
	dbFile := path + ".sqlite3"

	os.Remove(dbFile)
	defer os.Remove(dbFile)

	writer := datarecording.NewDataRecorder(path)
	assert.NotNil(t, writer, "DataRecorder should be created successfully")
	writer.Close()

	reader := datarecording.NewReader(dbFile)
	defer reader.Close()

	tableName := "exec_info"
	reader.MapTable(tableName, execInfo{})

	results, _, err := reader.Query(
		context.Background(), tableName, datarecording.QueryParams{})
	assert.NoError(t, err, "Should be able to query the database")

	assert.Len(t, results, 4, "Should have 4 execution info records")

	expectedProperties := []string{
		"Start Time",
		"Command",
		"Working Directory",
		"End Time",
	}
	actualProperties := make([]string, len(results))
	for i, result := range results {
		if execInfo, ok := result.(*execInfo); ok {
			actualProperties[i] = execInfo.Property
		}
	}
	assert.Equal(t, expectedProperties, actualProperties,
		"Should have the expected four properties in correct order")
}
