package data_recorder

import "github.com/sarchlab/akita/v4/tracing"

//DataRecorder is a backend that can record and store data
type DataRecorder interface {
	//Init establishes a connection to the database
	Init()

	//CreateTable creates a new table with given filename
	CreateTable(table string)

	//DataInsert writes task into given table
	DataInsert(table string, task tracing.Task)

	//ListComponents lists components in the given table
	ListComponents(table string)

	//ListTable returns a slice containing names of all tables
	ListTable()

	//ListTask lists tasks according to the given query
	ListTasks(query tracing.TaskQuery)

	//Flush flushes all the baffered task into database
	Flush()
}
