package analysis

// import (
// 	"database/sql"
// 	"fmt"
// 	"os"

// 	// Need to use MySQL connections.
// 	_ "github.com/mattn/go-sqlite3"
// 	"github.com/rs/xid"
// 	"github.com/tebeka/atexit"
// )

// // SQLiteTraceWriter is a writer that writes trace data to a SQLite database.
// type SQLiteTraceWriter struct {
// 	*sql.DB
// 	statement *sql.Stmt

// 	dbName             string
// 	buffersToWriteToDB []BufferTask
// 	portToWriteToDB    []PortTask

// 	batchSize int
// }

// // NewSQLiteTraceWriter creates a new SQLiteWriter.
// func NewSQLiteTraceWriter(path string) *SQLiteTraceWriter {
// 	w := &SQLiteTraceWriter{
// 		dbName:    path,
// 		batchSize: 100000,
// 	}

// 	atexit.Register(func() {
// 		w.FlushBuffer()
// 		w.FlushPort()
// 	})

// 	return w
// }

// // Init establishes a connection to the database.
// func (t *SQLiteTraceWriter) Init() {
// 	t.createDatabase()
// 	t.prepareStatement()
// }

// func (t *SQLiteTraceWriter) createDatabase() {
// 	if t.dbName == "" {
// 		t.dbName = "performance_analyzer_" + xid.New().String()
// 	}

// 	filename := t.dbName + ".sqlite3"
// 	_, err := os.Stat(filename)
// 	if err == nil {
// 		panic(fmt.Errorf("file %s already exists", filename))
// 	}

// 	fmt.Fprintf(os.Stderr, "Trace is Collected in Database: %s\n", filename)

// 	db, err := sql.Open("sqlite3", filename)
// 	if err != nil {
// 		panic(err)
// 	}

// 	t.DB = db

// 	t.createbufferTable()
// 	t.createportTable()
// }

// func (t *SQLiteTraceWriter) createbufferTable() {
// 	t.mustExecute(`
// 		create table trace
// 		(
// 			ID                          varchar(100) null,
// 			start_time                  float        null,
// 			end_time                    float        null
// 		);
// 	`)

// 	t.mustExecute(`create index trace_ID_uindex
// 		on trace (ID);
// 		`)

// 	t.mustExecute(`create index trace_start_time_index
// 		on trace (start_time);
// 		`)

// 	t.mustExecute(`create index trace_end_time_index
// 		on trace (end_time);
// 		`)
// }

// //create port table
// func (t *SQLiteTraceWriter) createportTable() {
// 	t.mustExecute(`
// 		create table trace
// 		(
// 			port_name   		varchar(100) null,
// 			start_time  		float null,
// 			end_time   			float null
// 		);
// 	`)

// 	t.mustExecute(`create index trace_port_name_uindex
// 		on trace (port_name);
// 		`)

// 	t.mustExecute(`create index trace_start_time_index
// 		on trace (start_time);
// 		`)

// 	t.mustExecute(`create index trace_end_time_index
// 		on trace (end_time);
// 		`)
// }

// // WriteBuffer writes a buffer task to the database.
// func (t *SQLiteTraceWriter) WriteBuffer(task BufferTask) {
// 	t.buffersToWriteToDB = append(t.buffersToWriteToDB, task)
// 	if len(t.buffersToWriteToDB) >= t.batchSize {
// 		t.FlushBuffer()
// 	}
// }

// // WritePort writes a port task to the database.
// func (t *SQLiteTraceWriter) WritePort(task PortTask) {
// 	t.portToWriteToDB = append(t.portToWriteToDB, task)
// 	if len(t.portToWriteToDB) >= t.batchSize {
// 		t.FlushPort()
// 	}
// }

// // Flush writes all the buffered tasks to the database.
// func (t *SQLiteTraceWriter) FlushBuffer() {
// 	if len(t.buffersToWriteToDB) == 0 && len(t.portToWriteToDB) == 0 {
// 		return
// 	}

// 	t.mustExecute("BEGIN TRANSACTION")
// 	defer t.mustExecute("COMMIT TRANSACTION")

// 	for _, task := range t.buffersToWriteToDB {
// 		// fmt.Println("inserting: ", task.ID)
// 		_, err := t.statement.Exec(
// 			task.ID,
// 			task.StartTime,
// 			task.EndTime,
// 		)
// 		if err != nil {
// 			fmt.Println(task)
// 			panic(err)
// 		}
// 	}

// 	t.buffersToWriteToDB = nil
// }

// //FlushPort writes all the buffered tasks to the database.
// func (t *SQLiteTraceWriter) FlushPort() {
// 	if len(t.portToWriteToDB) == 0 {
// 		return
// 	}

// 	t.mustExecute("BEGIN TRANSACTION")
// 	defer t.mustExecute("COMMIT TRANSACTION")

// 	for _, task := range t.portToWriteToDB {
// 		// fmt.Println("inserting: ", task.ID)
// 		_, err := t.statement.Exec(
// 			task.PortName,
// 			task.StartTime,
// 			task.EndTime,
// 		)
// 		if err != nil {
// 			fmt.Println(task)
// 			panic(err)
// 		}
// 	}
// }

// func (t *SQLiteTraceWriter) prepareStatement() {
// 	sqlStr := `INSERT INTO trace VALUES (?, ?, ?)`

// 	stmt, err := t.Prepare(sqlStr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	t.statement = stmt
// }

// func (t *SQLiteTraceWriter) mustExecute(query string) sql.Result {
// 	res, err := t.Exec(query)
// 	if err != nil {
// 		fmt.Printf("Failed to execute: %s\n", query)
// 		panic(err)
// 	}
// 	return res
// }

// // SQLiteTraceReader is a reader that reads trace data from a SQLite database.
// type SQLiteTraceReader struct {
// 	*sql.DB

// 	filename string
// }

// // NewSQLiteTraceReader creates a new SQLiteTraceReader.
// func NewSQLiteTraceReader(filename string) *SQLiteTraceReader {
// 	r := &SQLiteTraceReader{
// 		filename: filename,
// 	}

// 	return r
// }

// // Init establishes a connection to the database.
// func (r *SQLiteTraceReader) Init() {
// 	db, err := sql.Open("sqlite3", r.filename)
// 	if err != nil {
// 		panic(err)
// 	}

// 	r.DB = db
// }

// // ListComponents returns a list of components in the trace.
// func (r *SQLiteTraceReader) ListComponents() []string {
// 	var components []string

// 	rows, err := r.Query("SELECT DISTINCT location FROM trace")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer func() {
// 		err := rows.Close()
// 		if err != nil {
// 			panic(err)
// 		}
// 	}()

// 	for rows.Next() {
// 		var component string
// 		err := rows.Scan(&component)
// 		if err != nil {
// 			panic(err)
// 		}
// 		components = append(components, component)
// 	}

// 	return components
// }

// // ListTasks returns a list of tasks in the trace according to the given query.
// func (r *SQLiteTraceReader) ListTasks(query TaskQuery) []Task {
// 	sqlStr := r.prepareTaskQueryStr(query)

// 	rows, err := r.Query(sqlStr)
// 	if err != nil {
// 		panic(err)
// 	}

// 	tasks := []Task{}
// 	for rows.Next() {
// 		t := Task{}
// 		pt := Task{}

// 		if query.EnableParentTask {
// 			t.ParentTask = &pt
// 			err := rows.Scan(
// 				&t.ID,
// 				&t.ParentID,
// 				&t.Kind,
// 				&t.What,
// 				&t.Where,
// 				&t.StartTime,
// 				&t.EndTime,
// 				&pt.ID,
// 				&pt.ParentID,
// 				&pt.Kind,
// 				&pt.What,
// 				&pt.Where,
// 				&pt.StartTime,
// 				&pt.EndTime,
// 			)
// 			if err != nil {
// 				panic(err)
// 			}
// 		} else {
// 			err := rows.Scan(
// 				&t.ID,
// 				&t.Kind,
// 				&t.What,
// 				&t.Where,
// 				&t.StartTime,
// 				&t.EndTime,
// 			)
// 			if err != nil {
// 				panic(err)
// 			}
// 		}

// 		tasks = append(tasks, t)
// 	}

// 	return tasks
// }

// func (r *SQLiteTraceReader) prepareTaskQueryStr(query TaskQuery) string {
// 	sqlStr := `
// 		SELECT
// 			t.task_id,
// 			t.parent_id,
// 			t.kind,
// 			t.what,
// 			t.location,
// 			t.start_time,
// 			t.end_time
// 	`

// 	if query.EnableParentTask {
// 		sqlStr += `,
// 			pt.task_id,
// 			pt.parent_id,
// 			pt.kind,
// 			pt.what,
// 			pt.location,
// 			pt.start_time,
// 			pt.end_time
// 		`
// 	}

// 	sqlStr += `
// 		FROM trace t
// 	`

// 	if query.EnableParentTask {
// 		sqlStr += `
// 			LEFT JOIN trace pt
// 			ON t.parent_id = pt.task_id
// 		`
// 	}

// 	sqlStr = r.addQueryConditionsToQueryStr(sqlStr, query)

// 	return sqlStr
// }

// func (*SQLiteTraceReader) addQueryConditionsToQueryStr(
// 	sqlStr string,
// 	query TaskQuery,
// ) string {
// 	sqlStr += `
// 		WHERE 1=1
// 	`

// 	if query.ID != "" {
// 		sqlStr += `
// 			AND t.task_id = '` + query.ID + `'
// 		`
// 	}

// 	if query.ParentID != "" {
// 		sqlStr += `
// 			AND t.parent_id = '` + query.ParentID + `'
// 		`
// 	}

// 	if query.Kind != "" {
// 		sqlStr += `
// 			AND t.kind = '` + query.Kind + `'
// 		`
// 	}

// 	if query.Where != "" {
// 		sqlStr += `
// 			AND t.location = '` + query.Where + `'
// 		`
// 	}

// 	if query.EnableTimeRange {
// 		sqlStr += fmt.Sprintf(
// 			"AND t.end_time > %.15f AND t.start_time < %.15f",
// 			query.StartTime,
// 			query.EndTime)
// 	}

// 	return sqlStr
// }
