package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
)

// BlockedComponent is a component ranked by how long its tasks spent blocked.
type BlockedComponent struct {
	Component   string  `json:"component"`
	BlockedTime float64 `json:"blocked_time"`
}

// blockedQuery sums, per component, the time its tasks spent blocked. A
// milestone marks the release of a blocking condition, so the interval ending
// at it (from the previous step, or the task's start) is the blocked span. The
// "work" kind marks a productive interval, not a wait, so it is excluded.
const blockedQuery = `
WITH steps AS (
    SELECT m.TaskID, m.Time, m.Kind, t.Location, t.StartTime,
           LAG(m.Time) OVER (PARTITION BY m.TaskID ORDER BY m.Time) AS prev_time
    FROM milestone m
    JOIN trace t ON m.TaskID = t.ID
)
SELECT loc.Locale AS component,
       SUM(MAX(s.Time - COALESCE(s.prev_time, s.StartTime), 0)) AS blocked_time
FROM steps s
JOIN location loc ON s.Location = loc.ID
WHERE s.Kind <> 'work'
GROUP BY loc.Locale
ORDER BY blocked_time DESC`

// MostBlocked returns components ranked by total blocked time, most first. A
// trace without a milestone table (or one the query cannot run on) yields an
// empty slice rather than an error.
func (r *SQLiteTraceReader) MostBlocked(ctx context.Context) []BlockedComponent {
	result := []BlockedComponent{}

	rows, err := r.QueryContext(ctx, blockedQuery)
	if err != nil {
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var c BlockedComponent
		var blocked sql.NullFloat64
		if err := rows.Scan(&c.Component, &blocked); err != nil {
			continue
		}
		c.BlockedTime = blocked.Float64
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		// A mid-iteration error means the ranking may be truncated; surface it
		// in the log rather than presenting a partial result as complete.
		log.Printf("blocked-component query: %v", err)
	}

	return result
}

func (s *Server) httpBlocked(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, s.traceReader.MostBlocked(r.Context()))
}
