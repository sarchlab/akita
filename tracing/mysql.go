package tracing

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	// Need to use MySQL connections.
	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/xid"
	"github.com/tebeka/atexit"
)

// MySQLTraceWriter is a task tracer that can store the tasks into a MySQL
// database.
type MySQLTraceWriter struct {
	dbConnection
	tasksToWriteToDB []Task
	batchSize        int
}

// NewMySQLTraceWriter returns a new MySQLWriter.
// The init function must be called before using the backend.
func NewMySQLTraceWriter() *MySQLTraceWriter {
	t := &MySQLTraceWriter{
		batchSize: 100000,
	}

	atexit.Register(func() { t.Flush() })

	return t
}

// Init establishes a connection to MySQL and creates a database.
func (t *MySQLTraceWriter) Init() {
	t.dbConnection.init("")
	t.createDatabase()
}

func (t *MySQLTraceWriter) createDatabase() {
	dbName := "akita_trace_" + xid.New().String()
	t.dbName = dbName
	log.Printf("Trace is Collected in Database: %s\n", dbName)

	t.mustExecute("CREATE DATABASE " + dbName)
	t.mustExecute("USE " + dbName)

	t.createTable()
}

func (t *MySQLTraceWriter) createTable() {
	t.mustExecute(`
		create table trace
		(
			task_id    varchar(200) not null unique primary key,
			parent_id  varchar(200) null,
			kind       varchar(100) null,
			what       varchar(100) null,
			location   varchar(100) null,
			start_time float       null,
			end_time   float       null
		);
	`)

	t.mustExecute(`
        ALTER TABLE trace ENGINE=InnoDB;
	`)

	t.mustExecute(`
		create index trace_end_time_index
			on trace (end_time) USING BTREE;
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
			on trace (start_time) USING BTREE;
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

// Write writes the task into the database.
func (t *MySQLTraceWriter) Write(task Task) {
	t.tasksToWriteToDB = append(t.tasksToWriteToDB, task)
	if len(t.tasksToWriteToDB) > t.batchSize {
		t.Flush()
	}
}

// Flush writes all the tasks in the buffer into the database.
func (t *MySQLTraceWriter) Flush() {
	sqlStr := `INSERT INTO trace VALUES`
	vals := []interface{}{}

	for i := range t.tasksToWriteToDB {
		sqlStr += "(?, ?, ?, ?, ?, ?, ?),"
		vals = append(vals,
			t.tasksToWriteToDB[i].ID,
			t.tasksToWriteToDB[i].ParentID,
			t.tasksToWriteToDB[i].Kind,
			t.tasksToWriteToDB[i].What,
			t.tasksToWriteToDB[i].Where,
			t.tasksToWriteToDB[i].StartTime,
			t.tasksToWriteToDB[i].EndTime,
		)
	}

	sqlStr = strings.TrimSuffix(sqlStr, ",")
	// fmt.Println(sqlStr)
	stmt, err := t.Prepare(sqlStr)
	if err != nil {
		panic(err)
	}

	_, err = stmt.Exec(vals...)
	if err != nil {
		panic(err)
	}

	err = stmt.Close()
	if err != nil {
		panic(err)
	}

	t.tasksToWriteToDB = nil
}

// MySQLTraceReader can read tasks from a MySQL database.
type MySQLTraceReader struct {
	dbConnection

	dbName string
}

// NewMySQLTraceReader returns a new MySQLTraceReader.
// The Init function must be called before using the reader.
func NewMySQLTraceReader(dbName string) *MySQLTraceReader {
	r := &MySQLTraceReader{
		dbName: dbName,
	}

	return r
}

// Init establishes a connection to MySQL.
func (r *MySQLTraceReader) Init() {
	r.dbConnection.init(r.dbName)
}

// ListComponents returns a list of components in the trace.
func (r *MySQLTraceReader) ListComponents() []string {
	var components []string

	rows, err := r.Query("SELECT DISTINCT location FROM tasks")
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
func (r *MySQLTraceReader) ListTasks(query TaskQuery) []Task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}

	tasks := []Task{}
	for rows.Next() {
		t := Task{}
		pt := Task{}

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

		t.ParentTask = &pt

		tasks = append(tasks, t)
	}

	return tasks
}

func (r *MySQLTraceReader) prepareTaskQueryStr(query TaskQuery) string {
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

func (*MySQLTraceReader) addQueryConditionsToQueryStr(
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

type dbConnection struct {
	*sql.DB

	username  string
	password  string
	ipAddress string
	port      int
	dbName    string
}

func (c *dbConnection) init(dbName string) {
	c.dbName = dbName

	c.getCredentials()
	c.connect()
}

func (c *dbConnection) getCredentials() {
	c.username = os.Getenv("AKITA_TRACE_USERNAME")
	if c.username == "" {
		panic(`trace username is not set, use environment variable AKITA_TRACE_USERNAME to set it.`)
	}

	c.password = os.Getenv("AKITA_TRACE_PASSWORD")
	c.ipAddress = os.Getenv("AKITA_TRACE_IP")
	if c.ipAddress == "" {
		c.ipAddress = "127.0.0.1"
	}

	portString := os.Getenv("AKITA_TRACE_PORT")
	if portString == "" {
		portString = "3306"
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		panic(err)
	}
	c.port = port
}

func (c *dbConnection) connect() {
	connectStr := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		c.username, c.password, c.ipAddress, c.port, c.dbName)
	db, err := sql.Open("mysql", connectStr)
	if err != nil {
		panic(err)
	}

	c.DB = db
}

func (c *dbConnection) mustExecute(query string) sql.Result {
	res, err := c.Exec(query)
	if err != nil {
		panic(err)
	}

	return res
}
