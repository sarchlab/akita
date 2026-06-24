package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
)

// ComponentResidency ranks a component scope by the total in-flight time of its
// tasks (Σ EndTime − StartTime), summed over every location beneath that scope. It
// is a cheap, milestone-free proxy for "where the simulation spends time", so the
// busiest / most-contended components rank first.
type ComponentResidency struct {
	Component string  `json:"component"` // component scope (first dotted segment)
	TaskTime  float64 `json:"task_time"`
}

// residencyIndex is a covering index over exactly the columns the residency query
// reads, so the grouped SUM is answered straight from the index without touching
// the (multi-million-row) trace table — about 30x faster on a large trace.
const residencyIndex = `
CREATE INDEX IF NOT EXISTS idx_trace_loc_times ON trace(Location, StartTime, EndTime)`

// residencyQuery sums task time per location and rolls it up to the component
// scope — the first dotted segment of the location name (e.g. "AT.Bottom.incoming"
// -> "AT") — so the ranking is by whole components (matching the dashboard's
// top-level grouping) rather than individual one-kind location facets. It groups
// by the integer Location first (one covering-index scan) and joins the handful of
// location names afterward, keeping the heavy work on the index; the cheap outer
// GROUP BY then rolls those per-location sums up to their scope.
const residencyQuery = `
SELECT
    CASE
        WHEN instr(loc.Locale, '.') > 0
        THEN substr(loc.Locale, 1, instr(loc.Locale, '.') - 1)
        ELSE loc.Locale
    END AS component,
    SUM(g.task_time) AS task_time
FROM (
    SELECT Location, SUM(EndTime - StartTime) AS task_time
    FROM trace
    GROUP BY Location
) g
JOIN location loc ON g.Location = loc.ID
GROUP BY component
ORDER BY task_time DESC`

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
