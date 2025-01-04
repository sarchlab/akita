package tracing

import (
	"database/sql"
	"fmt"

	// SQLite driver
	_ "github.com/mattn/go-sqlite3"
)

type DataRecorderTraceReader struct {
	*sql.DB
}

func NewDataRecorderTraceReader(filename string) *DataRecorderTraceReader {
	db, err := sql.Open("sqlite3", filename)
	if err != nil {
		panic(err)
	}

	return &DataRecorderTraceReader{
		DB: db,
	}
}

func (r *DataRecorderTraceReader) ListComponents() []string {
	var components []string

	rows, err := r.Query("SELECT DISTINCT location FROM trace")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

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

func (r *DataRecorderTraceReader) ListTasks(query TaskQuery) []Task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.Query(sqlStr)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

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

func (r *DataRecorderTraceReader) prepareTaskQueryStr(query TaskQuery) string {
	sqlStr := `
		SELECT 
			t.task_id as id, 
			t.parent_id,
			t.kind,
			t.what,
			t.location as "where",
			t.start_time,
			t.end_time
	`

	if query.EnableParentTask {
		sqlStr += `,
			pt.task_id as parent_id,
			pt.parent_id as parent_parent_id,
			pt.kind as parent_kind,
			pt.what as parent_what,
			pt.location as parent_where,
			pt.start_time as parent_start_time,
			pt.end_time as parent_end_time
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

func (r *DataRecorderTraceReader) addQueryConditionsToQueryStr(
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
