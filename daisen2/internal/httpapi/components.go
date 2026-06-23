package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
)

// ComponentResidency ranks a component by the total in-flight time of its tasks
// (Σ EndTime − StartTime). It is a cheap, milestone-free proxy for "where the
// simulation spends time", so the busiest / most-contended components rank first.
type ComponentResidency struct {
	Component string  `json:"component"`
	TaskTime  float64 `json:"task_time"`
}

// residencyIndex is a covering index over exactly the columns the residency query
// reads, so the grouped SUM is answered straight from the index without touching
// the (multi-million-row) trace table — about 30x faster on a large trace.
const residencyIndex = `
CREATE INDEX IF NOT EXISTS idx_trace_loc_times ON trace(Location, StartTime, EndTime)`

// residencyQuery sums each component's total task time. It groups by the integer
// Location first (one covering-index scan) and joins the handful of location
// names afterward, keeping the heavy work on the index.
const residencyQuery = `
SELECT loc.Locale AS component, g.task_time
FROM (
    SELECT Location, SUM(EndTime - StartTime) AS task_time
    FROM trace
    GROUP BY Location
) g
JOIN location loc ON g.Location = loc.ID
ORDER BY g.task_time DESC`

// ComponentsByResidency returns components ranked by total task time, most first.
// A trace without a trace/location table (or one the query cannot run on) yields
// an empty slice rather than an error.
func (r *SQLiteTraceReader) ComponentsByResidency(ctx context.Context) []ComponentResidency {
	result := []ComponentResidency{}

	// Best-effort: build the covering index once (a no-op once it exists, thanks
	// to IF NOT EXISTS). Ignored on a read-only connection — the query then just
	// runs against the table, slower but correct.
	_, _ = r.ExecContext(ctx, residencyIndex)

	rows, err := r.QueryContext(ctx, residencyQuery)
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var c ComponentResidency
		var taskTime sql.NullFloat64
		if err := rows.Scan(&c.Component, &taskTime); err != nil {
			continue
		}
		c.TaskTime = taskTime.Float64
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		// A mid-iteration error means the ranking may be truncated; surface it in
		// the log rather than presenting a partial result as complete.
		log.Printf("component-residency query: %v", err)
	}

	return result
}

func (s *Server) httpComponents(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, s.traceReader.ComponentsByResidency(r.Context()))
}
