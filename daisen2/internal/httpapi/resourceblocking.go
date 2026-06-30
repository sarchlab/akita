package httpapi

import (
	"context"
	"net/http"
	"strconv"
)

// ResourceTimelineResponse is the occupancy of tasks blocked on a single hardware
// resource over a time range: at each bin, how many in-flight tasks are blocked
// waiting on it. The curve's buildup and fall is where the resource's contention
// forms and resolves — the same shaded-area method the task-count chart uses.
type ResourceTimelineResponse struct {
	What      string  `json:"what"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	NumBins   int     `json:"num_bins"`
	// Sample is the 1-in-N task stride used; >1 means the counts are estimates
	// (scaled back up) so a resource blocking hundreds of thousands of tasks still
	// returns promptly.
	Sample int `json:"sample"`
	// Total is the distinct tasks that ever block on this resource (whole trace).
	Total int `json:"total"`
	// Bins holds the per-bin count of tasks blocked on the resource.
	Bins []int `json:"bins"`
}

// resourceBlockingTaskBudget bounds the tasks an auto-sampled occupancy scan
// visits; a resource blocking more than this is strided 1-in-N so the scan stays
// responsive.
const resourceBlockingTaskBudget = 60_000

// ResourceBlockingOccupancy bins how many tasks are blocked on `what` (a
// hardware_resource milestone's name) in each time bin. It finds the resource's
// tasks via the covering index, LAG-derives each one's blocked interval (the span
// ending at the resource milestone), keeps only the resource's intervals, and
// bins their occupancy exactly like ComponentTimeline / BlockingReasonOccupancy.
func (r *SQLiteTraceReader) ResourceBlockingOccupancy( //nolint:funlen // one cohesive occupancy-binning SQL pipeline
	ctx context.Context, what string, start, end float64, numBins, sample int,
) ResourceTimelineResponse {
	resp := ResourceTimelineResponse{
		What: what, StartTime: start, EndTime: end, NumBins: numBins, Sample: 1, Bins: []int{},
	}
	if numBins < 1 || end <= start || what == "" {
		return resp
	}

	r.ensureIndex(ctx, "Building index idx_milestone_Kind_What",
		"CREATE INDEX IF NOT EXISTS idx_milestone_Kind_What ON milestone(Kind, What, TaskID)")
	r.ensureIndex(ctx, "Building index idx_milestone_TaskID_Time",
		"CREATE INDEX IF NOT EXISTS idx_milestone_TaskID_Time ON milestone(TaskID, Time)")

	var total int
	_ = r.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT TaskID) FROM milestone WHERE Kind = 'hardware_resource' AND What = ?`,
		what).Scan(&total)
	resp.Total = total

	if sample < 1 {
		sample = 1
	}
	// Auto-stride a heavily-blocking resource down to the task budget so it returns
	// in a moment; the uniform sample preserves the occupancy shape in expectation.
	if sample == 1 && total > resourceBlockingTaskBudget {
		sample = total/resourceBlockingTaskBudget + 1
	}
	resp.Sample = sample

	be := newBinExpr(numBins, start, end)
	sampleFilter := ""
	if sample > 1 {
		sampleFilter = " AND (t.rowid % " + strconv.Itoa(sample) + ") = 0"
	}

	sqlStr := `
		WITH bx AS (
			SELECT DISTINCT TaskID FROM milestone WHERE Kind = 'hardware_resource' AND What = ?
		),
		ivals AS MATERIALIZED (
			SELECT
				m.What AS w,
				COALESCE(
					LAG(m.Time) OVER (PARTITION BY m.TaskID ORDER BY m.Time),
					t.StartTime
				) AS lo,
				m.Time AS hi
			FROM bx
			JOIN trace t ON t.ID = bx.TaskID
			JOIN milestone m ON m.TaskID = t.ID
			WHERE 1 = 1` + sampleFilter + `
		),
		events AS (
			SELECT
				CASE WHEN d.delta = 1
					THEN MAX(0, MIN(` + be.numBins + ` - 1, ` + be.floorOf("lo") + `))
					ELSE ` + be.ceilOf("hi") + `
				END AS bin,
				'x' AS k,
				d.delta AS delta
			FROM ivals
			CROSS JOIN (SELECT 1 AS delta UNION ALL SELECT -1 AS delta) d
			WHERE w = ? AND hi > ` + be.startStr + ` AND lo < ` + be.endStr + `
		)
		SELECT bin, k, delta, COUNT(*) * ` + strconv.Itoa(sample) + ` AS c
		FROM events
		GROUP BY bin, k, delta`

	rows, err := r.QueryContext(ctx, sqlStr, what, what)
	if err != nil {
		return resp
	}
	defer rows.Close()

	_, bins, _ := accumulateBins(rows, numBins)
	resp.Bins = make([]int, numBins)
	for b := range bins {
		if len(bins[b]) > 0 {
			resp.Bins[b] = bins[b][0]
		}
	}
	return resp
}

// TasksBlockingOn hydrates up to `limit` tasks that block on `what` (a
// hardware_resource milestone's name) and overlap the time window, with their
// milestones — so the resource page can draw a per-task gantt highlighting each
// task's wait for the resource. Used only when the resource's task set is small.
func (r *SQLiteTraceReader) TasksBlockingOn(
	ctx context.Context, what string, start, end float64, limit int,
) []Task {
	if what == "" || limit < 1 {
		return []Task{}
	}
	r.ensureIndex(ctx, "Building index idx_milestone_Kind_What",
		"CREATE INDEX IF NOT EXISTS idx_milestone_Kind_What ON milestone(Kind, What, TaskID)")

	s := strconv.FormatFloat(start, 'f', -1, 64)
	e := strconv.FormatFloat(end, 'f', -1, 64)
	rows, err := r.QueryContext(ctx, `
		SELECT DISTINCT m.TaskID
		FROM milestone m
		JOIN trace t ON t.ID = m.TaskID
		WHERE m.Kind = 'hardware_resource' AND m.What = ?
			AND t.EndTime > `+s+` AND t.StartTime < `+e+`
		LIMIT `+strconv.Itoa(limit), what)
	if err != nil {
		return []Task{}
	}

	ids := []uint64{}
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return []Task{}
	}

	return r.ListTasks(ctx, TaskQuery{IDs: ids, EnableMilestones: true})
}

func (s *Server) httpResourceTasks(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}
	what := r.FormValue("what")
	start, err := strconv.ParseFloat(r.FormValue("starttime"), 64)
	if err != nil {
		http.Error(w, "invalid starttime", http.StatusBadRequest)
		return
	}
	end, err := strconv.ParseFloat(r.FormValue("endtime"), 64)
	if err != nil {
		http.Error(w, "invalid endtime", http.StatusBadRequest)
		return
	}
	limit := 200
	if v := r.FormValue("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}
	writeJSON(w, s.traceReader.TasksBlockingOn(r.Context(), what, start, end, limit))
}

func (s *Server) httpResourceBlocking(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	what := r.FormValue("what")
	start, err := strconv.ParseFloat(r.FormValue("starttime"), 64)
	if err != nil {
		http.Error(w, "invalid starttime", http.StatusBadRequest)
		return
	}
	end, err := strconv.ParseFloat(r.FormValue("endtime"), 64)
	if err != nil {
		http.Error(w, "invalid endtime", http.StatusBadRequest)
		return
	}

	numBins := 200
	if v := r.FormValue("num_bins"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			numBins = n
		}
	}
	if numBins > 2000 {
		numBins = 2000
	}

	sample := 1
	if v := r.FormValue("sample"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			sample = n
		}
	}

	writeJSON(w, s.traceReader.ResourceBlockingOccupancy(r.Context(), what, start, end, numBins, sample))
}
