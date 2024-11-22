package tracing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	// Need to use SQLite connections.
	_ "github.com/mattn/go-sqlite3"

	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// SQLiteTraceWriter is a writer that writes trace data to a SQLite database.
type SQLiteTraceWriter struct {
    *sql.DB
    statement *sql.Stmt
	delayStatement *sql.Stmt
	progressStatement *sql.Stmt
	dependencyStatement     *sql.Stmt

    dbName            string
    tasksToWriteToDB  []Task
	delaysToWriteToDB []DelayEvent
	progressesToWriteToDB []ProgressEvent
	dependenciesToWriteToDB []DependencyEvent
    batchSize         int
}


// NewSQLiteTraceWriter creates a new SQLiteWriter.
func NewSQLiteTraceWriter(path string) *SQLiteTraceWriter {
    w := &SQLiteTraceWriter{
        dbName:    path,
        batchSize: 100000,
    }

	atexit.Register(func() { w.Flush() })

    return w
}


// Init establishes a connection to the database.
func (t *SQLiteTraceWriter) Init() {
	fileName := xid.New().String()
	t.createDatabase(fileName)
	t.createTable()
	t.createDelayTable()
	t.createProgressTable()
	t.createDependencyTable()

	t.prepareStatement()
	t.prepareDelayStatement()
	t.prepareProgressStatement()
    t.prepareDependencyStatement()
}

// Write writes a task to the database.
func (t *SQLiteTraceWriter) Write(task Task) {
	fmt.Printf("Writing task ID: %s, Task Type: %s, Task Description: %s\n",
        task.ID, task.Kind, task.What)
	t.tasksToWriteToDB = append(t.tasksToWriteToDB, task)
	if len(t.tasksToWriteToDB) >= t.batchSize {
		t.Flush()
	}
}

// Flush writes all the buffered tasks to the database.
func (t *SQLiteTraceWriter) Flush() {
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

func (t *SQLiteTraceWriter) WriteDelay(event DelayEvent) {
    t.delaysToWriteToDB = append(t.delaysToWriteToDB, event)
    if len(t.delaysToWriteToDB) >= t.batchSize {
        t.FlushDelay()
    }
}


func (t *SQLiteTraceWriter) FlushDelay() {
    if len(t.delaysToWriteToDB) == 0 {
        return
    }

    t.mustExecute("BEGIN TRANSACTION")
    for _, event := range t.delaysToWriteToDB {
        _, err := t.delayStatement.Exec(event.EventID, event.TaskID, event.Type, event.What, event.Source, event.Time)
        if err != nil {
            fmt.Printf("Failed to insert delay event: %+v\n", event)
            panic(err)
        }
    }
    t.mustExecute("COMMIT TRANSACTION")
    t.delaysToWriteToDB = nil
}

func (t *SQLiteTraceWriter) WriteProgress(event ProgressEvent) {
    t.progressesToWriteToDB = append(t.progressesToWriteToDB, event)
    if len(t.progressesToWriteToDB) >= t.batchSize {
        t.FlushProgress()
    }
}


func (t *SQLiteTraceWriter) FlushProgress() {
    if len(t.progressesToWriteToDB) == 0 {
        return
    }

    t.mustExecute("BEGIN TRANSACTION")
    for _, event := range t.progressesToWriteToDB {
        _, err := t.progressStatement.Exec(event.ProgressID, event.TaskID, event.Source, event.Time, event.Reason)
        if err != nil {
            fmt.Printf("Failed to insert progress event: %+v\n", event)
            panic(err)
        }
    }
    t.mustExecute("COMMIT TRANSACTION")
    t.progressesToWriteToDB = nil
}


func (t *SQLiteTraceWriter) WriteDependency(event DependencyEvent) {
    // Convert the dependent IDs to JSON for storage
    dependentIDsJSON, err := json.Marshal(event.DependentID)
    if err != nil {
        panic(err) // Handle the error appropriately
    }

    // Set the JSON string in the event struct before buffering
    event.DependentIDJSON = string(dependentIDsJSON)

    t.dependenciesToWriteToDB = append(t.dependenciesToWriteToDB, event)
    if len(t.dependenciesToWriteToDB) >= t.batchSize {
        t.FlushDependency()
    }
}

func (t *SQLiteTraceWriter) FlushDependency() {
    if len(t.dependenciesToWriteToDB) == 0 {
        return
    }

    t.mustExecute("BEGIN TRANSACTION")
    for _, event := range t.dependenciesToWriteToDB {
        _, err := t.dependencyStatement.Exec(event.ProgressID, event.DependentIDJSON)
        if err != nil {
            fmt.Printf("Failed to insert dependency event: %+v\n", event)
            panic(err)
        }
    }
    t.mustExecute("COMMIT TRANSACTION")
    t.dependenciesToWriteToDB = nil
}




func (t *SQLiteTraceWriter) createDatabase(fileName string) {
	if t.dbName == "" {
		t.dbName = "akita_trace_" + fileName
	}

	filename := t.dbName + ".sqlite3"
	_, err := os.Stat(filename)
	fmt.Println("Original database opened successfully")
	if err == nil {
		panic(fmt.Errorf("file %s already exists", filename))
	}

	fmt.Fprintf(os.Stderr, "Trace is Collected in Database Jijie: %s\n", filename)

	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		fmt.Println("Original database open error \n" )
		panic(err)
	}

	t.DB = db
}

func (t *SQLiteTraceWriter) createTable() {
	t.mustExecute(`
		create table trace
		(
			task_id    varchar(200) not null default 'default_task_id',
			parent_id  varchar(200) default 'default_parent_id',
			kind       varchar(100) default 'default_kind',
			what       varchar(100) default 'default_what',
			location   varchar(100) default 'default_location',
			start_time float        not null,
			end_time   float        default 0
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

func (t *SQLiteTraceWriter) createDelayTable() {
	t.mustExecute(`
		CREATE TABLE IF NOT EXISTS delay
		(
			event_id VARCHAR(200) NULL,
			task_id  VARCHAR(200) NULL,
			type     VARCHAR(200) NULL,
			what     VARCHAR(200) NULL,
			source    VARCHAR(200) NULL,
			time     FLOAT NULL
		);
	`)
}

func (t *SQLiteTraceWriter) createProgressTable() {
	t.mustExecute(`
		CREATE TABLE IF NOT EXISTS progress
		(
			progress_id VARCHAR(200) NULL,
			task_id     VARCHAR(200) NULL,
			source      VARCHAR(200) NULL,
			time        FLOAT NULL,
			reason      VARCHAR(200) NULL
		);
	`)
}

func (t *SQLiteTraceWriter) createDependencyTable() {
    t.mustExecute(`
        CREATE TABLE IF NOT EXISTS dependency
        (
    		progress_id       VARCHAR(200) NOT NULL,
            dependent_id  TEXT NOT NULL
        );
    `)
}


func (t *SQLiteTraceWriter) prepareStatement() {
	fmt.Println("Preparing regular trace statement...")
	sqlStr := `INSERT INTO trace VALUES (?, ?, ?, ?, ?, ?, ?)`
	fmt.Printf("Inside original statement\n")

	stmt, err := t.Prepare(sqlStr)
	if err != nil {
		fmt.Printf("ERRRORRRR\n")
		panic(err)
	}

	t.statement = stmt
	fmt.Printf("Prepared original statement\n")
}

func (t *SQLiteTraceWriter) prepareDelayStatement() {
    sqlStr := `INSERT INTO delay (event_id, task_id, type, what, source, time) VALUES (?, ?, ?, ?, ?, ?)`
    stmt, err := t.Prepare(sqlStr)
    if err != nil {
        panic(err)
    }
    t.delayStatement = stmt
}

func (t *SQLiteTraceWriter) prepareProgressStatement() {
    sqlStr := `INSERT INTO progress (progress_id, task_id, source, time, reason) VALUES (?, ?, ?, ?, ?)`
    stmt, err := t.Prepare(sqlStr)
    if err != nil {
        panic(err)
    }
    t.progressStatement = stmt
}

func (t *SQLiteTraceWriter) prepareDependencyStatement() {
    sqlStr := `INSERT INTO dependency (progress_id, dependent_id) VALUES (?, ?)`
    stmt, err := t.Prepare(sqlStr)
    if err != nil {
        panic(err)
    }
    t.dependencyStatement = stmt
}


func (t *SQLiteTraceWriter) mustExecute(query string) sql.Result {
	res, err := t.Exec(query)
	if err != nil {
		fmt.Printf("Failed to execute: %s\n", query)
		panic(err)
	}
	return res
}

// SQLiteTraceReader is a reader that reads trace data from a SQLite database.
type SQLiteTraceReader struct {
	*sql.DB

	filename string
}

// NewSQLiteTraceReader creates a new SQLiteTraceReader.
func NewSQLiteTraceReader(filename string) *SQLiteTraceReader {
	r := &SQLiteTraceReader{
		filename: filename,
	}

	return r
}

// Init establishes a connection to the database.
func (r *SQLiteTraceReader) Init() {
	db, err := sql.Open("sqlite3", r.filename)
	if err != nil {
		panic(err)
	}

	r.DB = db
}

// ListComponents returns a list of components in the trace.
func (r *SQLiteTraceReader) ListComponents() []string {
	var components []string

	rows, err := r.Query("SELECT DISTINCT location FROM trace")
	if err != nil {
		panic(err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			panic(err)
		}
	}()

	for rows.Next() {
		var component string
		err := rows.Scan(&component)
		if err != nil {
			panic(err)
		}
		components = append(components, component)
	}

	return components
}

// ListTasks returns a list of tasks in the trace according to the given query.
func (r *SQLiteTraceReader) ListTasks(query TaskQuery) []Task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}

	tasks := []Task{}
	for rows.Next() {
		t := Task{}
		pt := Task{}

		if query.EnableParentTask {
			t.ParentTask = &pt
			err := rows.Scan(
				&t.ID,
				&t.ParentID,
				&t.Kind,
				&t.What,
				&t.Where,
				&t.StartTime,
				&t.EndTime,
				&pt.ID,
				&pt.ParentID,
				&pt.Kind,
				&pt.What,
				&pt.Where,
				&pt.StartTime,
				&pt.EndTime,
			)
			if err != nil {
				panic(err)
			}
		} else {
			err := rows.Scan(
				&t.ID,
				&t.ParentID,
				&t.Kind,
				&t.What,
				&t.Where,
				&t.StartTime,
				&t.EndTime,
			)
			if err != nil {
				panic(err)
			}
		}

		tasks = append(tasks, t)
	}

	return tasks
}

func (r *SQLiteTraceReader) prepareTaskQueryStr(query TaskQuery) string {
	sqlStr := `
		SELECT 
			t.task_id, 
			t.parent_id,
			t.kind,
			t.what,
			t.location,
			t.start_time,
			t.end_time
	`

	if query.EnableParentTask {
		sqlStr += `,
			pt.task_id,
			pt.parent_id,
			pt.kind,
			pt.what,
			pt.location,
			pt.start_time,
			pt.end_time
		`
	}

	sqlStr += `
		FROM trace t
	`

	if query.EnableParentTask {
		sqlStr += `
			LEFT JOIN trace pt
			ON t.parent_id = pt.task_id
		`
	}

	sqlStr = r.addQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

func (*SQLiteTraceReader) addQueryConditionsToQueryStr(
	sqlStr string,
	query TaskQuery,
) string {
	sqlStr += `
		WHERE 1=1
	`

	if query.ID != "" {
		sqlStr += `
			AND t.task_id = '` + query.ID + `'
		`
	}

	if query.ParentID != "" {
		sqlStr += `
			AND t.parent_id = '` + query.ParentID + `'
		`
	}

	if query.Kind != "" {
		sqlStr += `
			AND t.kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		sqlStr += `
			AND t.location = '` + query.Where + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND t.end_time > %.15f AND t.start_time < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}


func (r *SQLiteTraceReader) ListDelayEvents(query DelayQuery) []DelayEvent {
	sqlStr := r.prepareDelayQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			panic(err)
		}
	}()

	delayEvents := []DelayEvent{}
	for rows.Next() {
		var event DelayEvent
		err := rows.Scan(&event.EventID, &event.TaskID, &event.Type, &event.What, &event.Source, &event.Time)
		if err != nil {
			panic(err)
		}
		delayEvents = append(delayEvents, event)
	}

	return delayEvents
}

func (r *SQLiteTraceReader) prepareDelayQueryStr(query DelayQuery) string {
	sqlStr := `
		SELECT 
			event_id,
			task_id,
			type,
			what,
			source,
			time
		FROM delay
	`

	sqlStr = r.addDelayQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

func (r *SQLiteTraceReader) addDelayQueryConditionsToQueryStr(sqlStr string, query DelayQuery) string {
	sqlStr += `
		WHERE 1=1
	`

	if query.EventID != "" {
		sqlStr += `
			AND event_id = '` + query.EventID + `'
		`
	}

	if query.TaskID != "" {
		sqlStr += `
			AND task_id = '` + query.TaskID + `'
		`
	}

	if query.Type != "" {
		sqlStr += `
			AND type = '` + query.Type + `'
		`
	}

	if query.Source != "" {
		sqlStr += `
			AND source = '` + query.Source + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND time BETWEEN %.15f AND %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}



// ListProgressEvents returns a list of progress events from the progress table
func (r *SQLiteTraceReader) ListProgressEvents(query ProgressQuery) []ProgressEvent {
	sqlStr := r.prepareProgressQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			panic(err)
		}
	}()

	progressEvents := []ProgressEvent{}
	for rows.Next() {
		var event ProgressEvent
		err := rows.Scan(&event.ProgressID, &event.TaskID, &event.Source, &event.Time, &event.Reason)
		if err != nil {
			panic(err)
		}
		progressEvents = append(progressEvents, event)
	}

	return progressEvents
}

// prepareProgressQueryStr prepares the SQL query string for fetching progress events
func (r *SQLiteTraceReader) prepareProgressQueryStr(query ProgressQuery) string {
	sqlStr := `
		SELECT 
			progress_id,
			task_id,
			source,
			time,
			reason
		FROM progress
	`

	sqlStr = r.addProgressQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

// addProgressQueryConditionsToQueryStr adds conditions to the progress events SQL query string
func (r *SQLiteTraceReader) addProgressQueryConditionsToQueryStr(sqlStr string, query ProgressQuery) string {
	sqlStr += `
		WHERE 1=1
	`

	if query.TaskID != "" {
		sqlStr += `
			AND task_id = '` + query.TaskID + `'
		`
	}

	if query.Source != "" {
		sqlStr += `
			AND source = '` + query.Source + `'
		`
	}
	if query.Reason != "" {
		sqlStr += `
			AND reason = '` + query.Reason + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND time BETWEEN %.15f AND %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}


func (r *SQLiteTraceReader) ListDependencyEvents() []DependencyEvent {
	sqlStr := `
		SELECT 
			progress_id,
			dependent_id
		FROM dependency
	`

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := rows.Close()
		if err != nil {
			panic(err)
		}
	}()

	dependencyEvents := []DependencyEvent{}
	for rows.Next() {
		var event DependencyEvent
		err := rows.Scan(&event.ProgressID, &event.DependentIDJSON)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal([]byte(event.DependentIDJSON), &event.DependentID)
		if err != nil {
			panic(err)
		}
		dependencyEvents = append(dependencyEvents, event)
	}

	return dependencyEvents
}
