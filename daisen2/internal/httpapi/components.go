package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strings"
)

// ComponentResidency ranks a component scope by the total in-flight time of its
// tasks (Σ EndTime − StartTime), summed over every location beneath that scope. It
// is a cheap, milestone-free proxy for "where the simulation spends time", so the
// busiest / most-contended components rank first.
type ComponentResidency struct {
	Component string  `json:"component"` // component scope (first dotted segment)
	TaskTime  float64 `json:"task_time"`
	// Kinds is the set of distinct task kinds present under this scope, so a
	// client can pick metrics that fit what the component does — a
	// request-fulfilling component (req_in), an executor (req_out), or a
	// network component (flit/flit_e2e/msg_e2e), whose request-based metrics
	// would otherwise read zero.
	Kinds []string `json:"kinds"`
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

// componentKindsQuery lists, per component scope, the distinct task kinds present
// beneath it. Distinct (Location, Kind) pairs are few (one row per facet × kind),
// so this is cheap; it rolls them up to the same first-dotted-segment scope the
// residency query uses.
const componentKindsQuery = `
SELECT
    CASE
        WHEN instr(loc.Locale, '.') > 0
        THEN substr(loc.Locale, 1, instr(loc.Locale, '.') - 1)
        ELSE loc.Locale
    END AS component,
    GROUP_CONCAT(DISTINCT lk.Kind) AS kinds
FROM (SELECT DISTINCT Location, Kind FROM trace) lk
JOIN location loc ON lk.Location = loc.ID
GROUP BY component`

// componentKinds returns, per component scope, the set of distinct task kinds
// present beneath it. It is best-effort: a trace it cannot query yields an empty
// map.
func (r *SQLiteTraceReader) componentKinds(ctx context.Context) map[string][]string {
	kinds := map[string][]string{}

	rows, err := r.QueryContext(ctx, componentKindsQuery)
	if err != nil {
		return kinds
	}
	defer rows.Close()

	for rows.Next() {
		var component string
		var concat sql.NullString
		if err := rows.Scan(&component, &concat); err != nil {
			continue
		}
		if concat.Valid && concat.String != "" {
			kinds[component] = strings.Split(concat.String, ",")
		}
	}

	return kinds
}

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

	kindsByComp := r.componentKinds(ctx)
	for i := range result {
		if k := kindsByComp[result[i].Component]; k != nil {
			result[i].Kinds = k
		} else {
			result[i].Kinds = []string{}
		}
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
