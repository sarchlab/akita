package datarecording_test

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v4/datarecording"
)

type Task struct {
	ID   int
	Name string
}

func Example() {
	dbPath := "test"
	recorder := datarecording.New(dbPath)

	cleanup := func() {
		os.Remove(dbPath + ".sqlite3")
	}
	defer cleanup()

	task1 := Task{1, "task1"}
	recorder.CreateTable("test_table", task1)

	task2 := Task{2, "task2"}
	recorder.InsertData("test_table", task2)
	recorder.Flush()

	tables := recorder.ListTables()
	fmt.Printf("The stored table: %s\n", tables[0])

	db, err := sql.Open("sqlite3", dbPath+".sqlite3")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT * FROM test_table")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var name string

		err = rows.Scan(&id, &name)
		if err != nil {
			panic(err)
		}

		fmt.Printf("ID: %d, Name: %s\n", id, name)
	}

	// Output:
	// The stored table: test_table
	// ID: 2, Name: task2
}
