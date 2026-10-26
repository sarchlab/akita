package datarecording_test

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/sim/datarecording"
)

type Task struct {
	ID    int    `json:"id" akita_data:"unique"`
	Name  string `json:"name" akita_data:"index"`
	Age   int    `json:"age" akita_data:"ignore"`
	Place string `json:"place" akita_data:"location"`
}

func Example() {
	dir, err := os.MkdirTemp("", "akita-recording-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "recording.sqlite3")
	recorder := datarecording.NewDataRecorder(dbPath)

	task1 := Task{1, "task1", 30, "A"}
	recorder.CreateTable("test_table", task1)

	task2 := Task{2, "task2", 15, "B"}
	recorder.InsertData("test_table", task2)
	recorder.Flush()

	recorder.Close()

	// Read recordings with SQLite directly, or use Daisen for trace analysis.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var task Task
	err = db.QueryRow(`SELECT t.ID, t.Name, l.Locale FROM test_table t
   JOIN akita$location l ON t.Place = l.ID`).Scan(&task.ID, &task.Name, &task.Place)
	if err != nil {
		panic(err)
	}
	fmt.Printf("ID: %d, Name: %s, Place: %s\n", task.ID, task.Name, task.Place)

	// Output:
	// ID: 2, Name: task2, Place: B
}
