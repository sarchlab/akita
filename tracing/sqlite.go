package tracing

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	// Need to use MySQL connections.
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// SQLiteWriter is a writer that writes trace data to a SQLite database.
type SQLiteWriter struct {
	*sync.Mutex
	*sql.DB
	statement *sql.Stmt

	dbName           string
	tasksToWriteToDB []Task
	taskByID         map[string]Task
	batchSize        int
}

// NewSQLiteWriter creates a new SQLiteWriter.
func NewSQLiteWriter() *SQLiteWriter {
	w := &SQLiteWriter{
		Mutex:     &sync.Mutex{},
		batchSize: 100000,
		taskByID:  make(map[string]Task),
	}

	atexit.Register(func() { w.Flush() })

	return w
}

// Init establishes a connection to the database.
func (t *SQLiteWriter) Init() {
	t.createDatabase()
	t.prepareStatement()
}

// Write writes a task to the database.
func (t *SQLiteWriter) Write(task Task) {
	if _, ok := t.taskByID[task.ID]; ok {
		panic(fmt.Sprintf("task %s already exists", task.ID))
	}

	t.taskByID[task.ID] = task

	t.tasksToWriteToDB = append(t.tasksToWriteToDB, task)
	if len(t.tasksToWriteToDB) >= t.batchSize {
		t.Flush()
	}
}

// Flush writes all the buffered tasks to the database.
func (t *SQLiteWriter) Flush() {
	t.Lock()
	defer t.Unlock()

	if len(t.tasksToWriteToDB) == 0 {
		return
	}

	t.mustExecute("BEGIN TRANSACTION")
	defer t.mustExecute("COMMIT TRANSACTION")

	for _, task := range t.tasksToWriteToDB {
		// fmt.Println("inserting: ", task.ID)
		_, err := t.statement.Exec(
			task.ID,
			task.ParentID,
			task.Kind,
			task.What,
			task.Where,
			task.StartTime,
			task.EndTime,
		)
		if err != nil {
			fmt.Println(task)
			panic(err)
		}
	}

	t.tasksToWriteToDB = nil
}

func (t *SQLiteWriter) createDatabase() {
	dbName := "akita_trace_" + xid.New().String()
	t.dbName = dbName
	fmt.Fprintf(os.Stderr, "Trace is Collected in Database: %s\n", dbName)

	db, err := sql.Open("sqlite3", "./"+dbName+".sqlite3")
	if err != nil {
		panic(err)
	}

	t.DB = db

	t.createTable()
}

func (t *SQLiteWriter) createTable() {
	t.mustExecute(`
		create table trace
		(
			task_id    varchar(200) null unique,
			parent_id  varchar(200) null,
			kind       varchar(100) null,
			what       varchar(100) null,
			location   varchar(100) null,
			start_time float        null,
			end_time   float        null
		);
	`)

	t.mustExecute(`
		create index trace_end_time_index
			on trace (end_time);
	`)

	t.mustExecute(`
		create index trace_task_id_uindex
			on trace (task_id);
	`)

	t.mustExecute(`
		create index trace_kind_index
			on trace (kind);
	`)

	t.mustExecute(`
		create index trace_start_time_index
			on trace (start_time);
	`)

	t.mustExecute(`
		create index trace_what_index
			on trace (what);
	`)

	t.mustExecute(`
		create index trace_location_index
			on trace (location);
	`)

	t.mustExecute(`
		create index trace_parent_id_index
			on trace (parent_id);
	`)
}

func (t *SQLiteWriter) prepareStatement() {
	sqlStr := `INSERT INTO trace VALUES (?, ?, ?, ?, ?, ?, ?)`

	stmt, err := t.Prepare(sqlStr)
	if err != nil {
		panic(err)
	}

	t.statement = stmt
}

func (t *SQLiteWriter) mustExecute(query string) sql.Result {
	res, err := t.Exec(query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(err)
	}
	return res
}
