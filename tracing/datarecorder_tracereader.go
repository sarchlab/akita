package tracing

import (
	"database/sql"
	"fmt"

	// SQLite driver
	_ "github.com/mattn/go-sqlite3"
	"github.com/sarchlab/akita/v4/sim"
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
	rows, err := r.DB.Query(`
        SELECT DISTINCT "Where" 
        FROM trace 
        WHERE "Where" IS NOT NULL
    `)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var components []string
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
	args := make([]interface{}, 0)
	conditions := make([]string, 0)

	sqlStr := `SELECT ID, ParentID, Kind, What, "Where", StartTime, EndTime FROM trace WHERE 1=1`

	if query.ID != "" {
		conditions = append(conditions, "ID = ?")
		args = append(args, query.ID)
	}

	if query.ParentID != "" {
		conditions = append(conditions, "ParentID = ?")
		args = append(args, query.ParentID)
	}

	if query.Kind != "" {
		conditions = append(conditions, "Kind = ?")
		args = append(args, query.Kind)
	}

	if query.Where != "" {
		conditions = append(conditions, `"Where" = ?`)
		args = append(args, query.Where)
	}

	if query.EnableTimeRange {
		conditions = append(conditions, "EndTime > ? AND StartTime < ?")
		args = append(args, query.StartTime, query.EndTime)
	}

	for _, condition := range conditions {
		sqlStr += " AND " + condition
	}

	sqlStr += " ORDER BY StartTime"

	rows, err := r.DB.Query(sqlStr, args...)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		err := rows.Scan(
			&task.ID,
			&task.ParentID,
			&task.Kind,
			&task.What,
			&task.Where,
			&task.StartTime,
			&task.EndTime,
		)
		if err != nil {
			panic(err)
		}

		milestoneRows, err := r.DB.Query(`
            SELECT ID, BlockingCategory, BlockingReason, BlockingLocation, Time
            FROM trace_milestones
            WHERE TaskID = ? AND Time >= ? AND Time <= ?
            ORDER BY Time`,
			task.ID, task.StartTime, task.EndTime)
		if err != nil {
			panic(err)
		}
		defer milestoneRows.Close()

		for milestoneRows.Next() {
			var (
				id, category, reason, location string
				timestamp                      float64
			)
			err := milestoneRows.Scan(&id, &category, &reason, &location, &timestamp)
			if err != nil {
				panic(err)
			}

			task.Steps = append(task.Steps, TaskStep{
				Time: sim.VTimeInSec(timestamp),
				What: fmt.Sprintf("%s: %s at %s", category, reason, location),
			})
		}

		milestoneRows.Close()

		if query.EnableParentTask && task.ParentID != "" {
			parentTask, err := r.getParentTask(task.ParentID)
			if err == nil {
				task.ParentTask = parentTask
			}
		}

		tasks = append(tasks, task)
	}
	return tasks
}

func (r *DataRecorderTraceReader) getParentTask(parentID string) (*Task, error) {
	var task Task
	err := r.DB.QueryRow(`
        SELECT ID, ParentID, Kind, What, "Where", StartTime, EndTime
        FROM trace
        WHERE ID = ?`,
		parentID).Scan(
		&task.ID,
		&task.ParentID,
		&task.Kind,
		&task.What,
		&task.Where,
		&task.StartTime,
		&task.EndTime,
	)
	if err != nil {
		return nil, err
	}
	return &task, nil
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
			AND ID = '` + query.ID + `'
		`
	}

	if query.ParentID != "" {
		sqlStr += `
			AND ParentID = '` + query.ParentID + `'
		`
	}

	if query.Kind != "" {
		sqlStr += `
			AND Kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		sqlStr += `
			AND Where = '` + query.Where + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND EndTime > %.15f AND StartTime < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}
