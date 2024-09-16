package datarecording_test

import (
	"fmt"
	"os"

	"github.com/sarchlab/akita/v4/datarecording"
)

type Task struct {
	id   int
	name string
}

func Example() {
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
	defer cleanup()

	task1 := Task{1, "task1"}
	writer.CreateTable("test_table", task1)

	task2 := Task{2, "task2"}
	writer.InsertData("test_table", task2)

	tables := reader.ListTables()
	fmt.Printf("The stored table: %s", tables)

	// Output:
	// Table test_table created successfully
	// Data is successfully inserted
	// The stored table: [test_table]
}
