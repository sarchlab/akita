package datarecording_test

import (
	"context"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v4/datarecording"
)

type Task struct {
	ID    int    `json:"id" akita_data:"unique"`
	Name  string `json:"name" akita_data:"index"`
	Age   int    `json:"age" akita_data:"ignore"`
	Place string `json:"place" akita_data:"location"`
}

func Example() {
	dbPath := "test"
	os.Remove(dbPath + ".sqlite3")

	recorder := datarecording.NewDataRecorder(dbPath)

	cleanup := func() {
		os.Remove(dbPath + ".sqlite3")
	}
	defer cleanup()

	task1 := Task{1, "task1", 30, "A"}
	recorder.CreateTable("test_table", task1)

	task2 := Task{2, "task2", 15, "B"}
	recorder.InsertData("test_table", task2)
	recorder.Flush()

	recorder.Close()

	reader := datarecording.NewReader(dbPath + ".sqlite3")
	reader.MapTable("test_table", Task{})

	results, _, err := reader.Query(context.Background(), "test_table", datarecording.QueryParams{})
	if err != nil {
		panic(err)
	}

	for _, result := range results {
		task := result.(*Task)
		fmt.Printf("ID: %d, Name: %s, Place: %s\n",
			task.ID, task.Name, task.Place)
	}

	reader.Close()

	// Output:
	// ID: 2, Name: task2, Place: B
}
