package datarecording_test

import (
	"context"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v5/datarecording"
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

	recorder, err := datarecording.NewDataRecorder(dbPath)
	if err != nil {
		panic(err)
	}

	cleanup := func() {
		os.Remove(dbPath + ".sqlite3")
	}
	defer cleanup()

	task1 := Task{1, "task1", 30, "A"}
	datarecording.MustRecord(recorder.CreateTable("test_table", task1))

	task2 := Task{2, "task2", 15, "B"}
	datarecording.MustRecord(recorder.InsertData("test_table", task2))
	datarecording.MustRecord(recorder.Flush())

	recorder.Close()

	reader, err := datarecording.NewReader(dbPath + ".sqlite3")
	if err != nil {
		panic(err)
	}
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
