package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sarchlab/akita/v4/tracing"
)

type Milestone struct {
	ID               string  `json:"id"`
	TaskID           string  `json:"task_id"`
	BlockingCategory string  `json:"blocking_category"`
	BlockingReason   string  `json:"blocking_reason"`
	BlockingLocation string  `json:"blocking_location"`
	Time             float64 `json:"time"`
}

type TaskWithMilestones struct {
	tracing.Task
	Milestones []Milestone `json:"milestones,omitempty"`
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

	query := tracing.TaskQuery{
		ID:               r.FormValue("id"),
		ParentID:         r.FormValue("parentid"),
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
	}

	tasks := traceReader.ListTasks(query)

	tasksWithMilestones := make([]TaskWithMilestones, len(tasks))

	sqliteReader, ok := traceReader.(*tracing.DataRecorderTraceReader)
	if !ok {
		panic("Unsupported trace reader type")
	}

	for i, task := range tasks {
		tasksWithMilestones[i].Task = task

		rows, err := sqliteReader.DB.Query(`
            SELECT ID, BlockingCategory, BlockingReason, BlockingLocation, Time
			FROM trace_milestones
			WHERE ID = ?
			AND Time >= ?
			AND Time <= ?
			ORDER BY Time`,
			task.ID, task.StartTime, task.EndTime)

		if err != nil {
			panic(err)
		}
		defer rows.Close()

		var milestones []Milestone
		for rows.Next() {
			var m Milestone
			err := rows.Scan(&m.ID, &m.BlockingCategory, &m.BlockingReason, &m.BlockingLocation, &m.Time)
			if err != nil {
				panic(err)
			}
			m.TaskID = task.ID
			milestones = append(milestones, m)
		}
		tasksWithMilestones[i].Milestones = milestones
	}

	rsp, err := json.Marshal(tasksWithMilestones)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}
