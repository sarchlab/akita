package datarecording_test

import (
	"fmt"
	"os"

	"github.com/sarchlab/akita/v4/datarecording"
)

type Task struct {
	ID   int    `json:"id" akita_data:"unique"`
	Name string `json:"name" akita_data:"index"`
	Age  int    `json:"age" akita_data:"ignore"`
}

func Example() {
	dbPath := "test"
	os.Remove(dbPath + ".sqlite3")

	recorder := datarecording.NewDataRecorder(dbPath)

	cleanup := func() {
		os.Remove(dbPath + ".sqlite3")
	}
	defer cleanup()

	task1 := Task{1, "task1", 30}
	recorder.CreateTable("test_table", task1)

	task2 := Task{2, "task2", 15}
	recorder.InsertData("test_table", task2)
	recorder.Flush()

	tables := recorder.ListTables()
	fmt.Printf("The stored table: %s\n", tables[1])

	recorder.Close()

	reader := datarecording.NewReader(dbPath + ".sqlite3")
	reader.MapTable("test_table", Task{})

	tables = reader.ListTables()
	fmt.Printf("The stored table: %s\n", tables[0])

	results, _, err := reader.Query("test_table", datarecording.QueryParams{})
	if err != nil {
		panic(err)
	}

	for _, result := range results {
		task := result.(*Task)
		fmt.Printf("ID: %d, Name: %s\n", task.ID, task.Name)
	}

	reader.Close()

	// Output:
	// The stored table: test_table
	// The stored table: test_table
	// ID: 2, Name: task2
}
