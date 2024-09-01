package datarecording_test

import (
	"os"
	"testing"

	"github.com/sarchlab/akita/v4/datarecording"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*datarecording.SQLiteWriter, *datarecording.SQLiteReader, func()) {
	dbPath := "test"
	writer := datarecording.NewSQLiteWriter(dbPath)
	writer.Init()

	reader := datarecording.NewSQLiteReader(dbPath)
	reader.Init()

	cleanup := func() {
		writer.DB.Close()
		reader.DB.Close()
		os.Remove(dbPath + ".sqlite3")
	}

	return writer, reader, cleanup
}

func TestSQLiteWriter_Init(t *testing.T) {
	writer, _, cleanup := setupTestDB(t)
	defer cleanup()

	assert.NotNil(t, writer.DB, "Database connection should be established")
}

func TestSQLiteWriter_CreateTable(t *testing.T) {
	writer, _, cleanup := setupTestDB(t)
	defer cleanup()

	task := struct {
		ID   int
		Name string
	}{}

	writer.CreateTable("test_table", task)

	var tableName string
	err := writer.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='test_table';").Scan(&tableName)
	require.NoError(t, err, "Table should be created")
	assert.Equal(t, "test_table", tableName, "Table name should match")
}

func TestSQLiteWriter_DataInsert(t *testing.T) {
	writer, _, cleanup := setupTestDB(t)
	defer cleanup()

	task := struct {
		ID   int
		Name string
	}{}
	writer.CreateTable("test_table", task)

	task1 := struct {
		ID   int
		Name string
	}{1, "Task1"}

	writer.InsertData("test_table", task1)
	writer.Flush()

	var id int
	var name string
	err := writer.QueryRow("SELECT ID, Name FROM test_table WHERE ID=1;").Scan(&id, &name)
	require.NoError(t, err, "Data should be inserted")
	assert.Equal(t, 1, id, "ID should match")
	assert.Equal(t, "Task1", name, "Name should match")
}

func TestSQLiteReader_Init(t *testing.T) {
	_, reader, cleanup := setupTestDB(t)
	defer cleanup()

	assert.NotNil(t, reader.DB, "Database connection should be established")
}

func TestSQLiteReader_ListTables(t *testing.T) {
	writer, reader, cleanup := setupTestDB(t)
	defer cleanup()

	task := struct {
		ID   int
		Name string
	}{}
	writer.CreateTable("test_table", task)

	tables := reader.ListTables()
	assert.Contains(t, tables, "test_table", "Table list should contain created table")
}

func TestSQLiteWriter_Flush(t *testing.T) {
	writer, _, cleanup := setupTestDB(t)
	defer cleanup()

	task := struct {
		ID   int
		Name string
	}{}
	writer.CreateTable("test_table", task)

	task1 := struct {
		ID   int
		Name string
	}{1, "Task1"}
	writer.InsertData("test_table", task1)

	writer.Flush()

	var id int
	var name string
	err := writer.QueryRow("SELECT ID, Name FROM test_table WHERE ID=1;").Scan(&id, &name)
	require.NoError(t, err, "Data should be flushed")
	assert.Equal(t, 1, id, "ID should match")
	assert.Equal(t, "Task1", name, "Name should match")
}

// This test should fail
func TestSQLiteWriter_BlockComplexStructs(t *testing.T) {
	writer, _, cleanup := setupTestDB(t)
	defer cleanup()

	type Attribute struct {
		ID int
	}

	task := struct {
		attribute Attribute
	}{}

	writer.CreateTable("test_table", task)
}
