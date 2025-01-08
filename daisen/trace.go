package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	// Need to use SQLite connections.
	_ "github.com/mattn/go-sqlite3"
)

type task struct {
	ID        string  `json:"id"`
	ParentID  string  `json:"parent_id"`
	Kind      string  `json:"kind"`
	What      string  `json:"what"`
	Where     string  `json:"where"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`

	ParentTask *task `json:"parent_task"`
}

// taskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type taskQuery struct {
	// Use ID to select a single task by its ID.
	ID string

	// Use ParentID to select all the tasks that are children of a task.
	ParentID string

	// Use Kind to select all the tasks that are of a kind.
	Kind string

	// Use Where to select all the tasks that are executed at a location.
	Where string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select tasks that overlaps with the given task range.
	StartTime, EndTime float64

	// EnableParentTask will also query the parent task of the selected tasks.
	EnableParentTask bool
}

type traceReader struct {
	*sql.DB

	filename string
}

// newTraceReader creates a new traceReader.
func newTraceReader(filename string) *traceReader {
	r := &traceReader{
		filename: filename,
	}

	return r
}

// Init establishes a connection to the database.
func (r *traceReader) Init() {
	db, err := sql.Open("sqlite3", r.filename)
	if err != nil {
		panic(err)
	}

	r.DB = db
}

// ListComponents returns a list of components in the trace.
func (r *traceReader) ListComponents() []string {
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
func (r *traceReader) ListTasks(query taskQuery) []task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}

	tasks := []task{}
	for rows.Next() {
		t := task{}
		pt := task{}

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

func (r *traceReader) prepareTaskQueryStr(query taskQuery) string {
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

func (r *traceReader) addQueryConditionsToQueryStr(
	sqlStr string,
	query taskQuery,
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

func httpTrace(w http.ResponseWriter, r *http.Request) {
	useTimeRange := true
	if r.FormValue("starttime") == "" || r.FormValue("endtime") == "" {
		useTimeRange = false
	}

	var err error

	startTime := 0.0
	endTime := 0.0

	if useTimeRange {
		startTime, err = strconv.ParseFloat(r.FormValue("starttime"), 64)
		if err != nil {
			panic(err)
		}

		endTime, err = strconv.ParseFloat(r.FormValue("endtime"), 64)
		if err != nil {
			panic(err)
		}
	}

	query := taskQuery{
		ID:               r.FormValue("id"),
		ParentID:         r.FormValue("parentid"),
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
	}

	tasks := reader.ListTasks(query)

	rsp, err := json.Marshal(tasks)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}
