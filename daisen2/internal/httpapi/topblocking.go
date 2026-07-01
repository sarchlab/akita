package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
)

// BlockingResource is one row of the "top blocking resources" ranking: a
// hardware-resource name (a hardware_resource milestone's What) and how much it
// blocked tasks.
type BlockingResource struct {
	// What is the resource name carried by the milestone (e.g. "vmem-inflight" or
	// "GPU[1].SA[9].L1ICache.trans").
	What string `json:"what"`
	// Count is the number of blocking events on this resource — hardware_resource
	// milestones naming it, i.e. how many times a task was released from blocking
	// on it. This is the ranking metric.
	Count int `json:"count"`
	// TaskCount is the distinct tasks that blocked on this resource (its breadth of
	// impact), shown alongside the count.
	TaskCount int `json:"task_count"`
}

// TopBlockingResourcesResponse ranks hardware resources most-blocking first.
type TopBlockingResourcesResponse struct {
	Resources []BlockingResource `json:"resources"`
}

// TopBlockingResources ranks the hardware resources that blocked tasks the most,
// by the number of blocking events (hardware_resource milestones) naming each.
// For a hardware_resource milestone, What is the resource name. A covering index
// over (Kind, What, TaskID) turns this into an index-only aggregation — sub-second
// even with tens of millions of milestones, with no window function or trace join.
// An empty scope ranks the whole trace (the index-page overview); a non-empty
// scope restricts to that location subtree.
func (r *SQLiteTraceReader) TopBlockingResources(
	ctx context.Context, scope string, limit int,
) TopBlockingResourcesResponse {
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	resp := TopBlockingResourcesResponse{Resources: []BlockingResource{}}

	// Covering index over the columns this aggregation reads, so it scans only the
	// hardware_resource entries in (What, TaskID) order instead of the whole table.
	r.ensureIndex(ctx, "Building index idx_milestone_Kind_What",
		"CREATE INDEX IF NOT EXISTS idx_milestone_Kind_What ON milestone(Kind, What, TaskID)")

	id := r.activity.Begin("query", "Ranking top blocking resources", scopeLabel(scope))
	defer r.activity.End(id)

	rows, err := r.queryTopBlocking(context.WithoutCancel(ctx), scope, limit)
	if err != nil {
		return resp
	}
	defer rows.Close()

	for rows.Next() {
		var br BlockingResource
		if err := rows.Scan(&br.What, &br.Count, &br.TaskCount); err != nil {
			continue
		}
		resp.Resources = append(resp.Resources, br)
	}
	return resp
}

func scopeLabel(scope string) string {
	if scope == "" {
		return "all components"
	}
	return scope
}

// queryTopBlocking runs the ranking SQL. With no scope it is a pure milestone
// aggregation (the fast index-only path); with a scope it joins trace to keep only
// the milestones of tasks in that location subtree.
func (r *SQLiteTraceReader) queryTopBlocking(
	ctx context.Context, scope string, limit int,
) (*sql.Rows, error) {
	if scope == "" {
		return r.QueryContext(ctx, `
			SELECT What, COUNT(*) AS cnt, COUNT(DISTINCT TaskID) AS tasks
			FROM milestone
			WHERE Kind = 'hardware_resource'
			GROUP BY What
			ORDER BY cnt DESC
			LIMIT `+strconv.Itoa(limit))
	}

	// Scoped: the location index prunes to the subtree's tasks before joining their
	// milestones. (idx_trace_loc_time_id is shared with the timeline charts.)
	r.ensureIndex(ctx, "Building index idx_trace_loc_time_id",
		`CREATE INDEX IF NOT EXISTS idx_trace_loc_time_id `+
			`ON trace(Location, StartTime, EndTime, ID)`)
	lo, hi := scopePrefixBounds(scope)
	return r.QueryContext(ctx, `
		WITH scope_locs AS (
			SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?)
		)
		SELECT m.What AS what, COUNT(*) AS cnt, COUNT(DISTINCT m.TaskID) AS tasks
		FROM trace t
		JOIN milestone m ON m.TaskID = t.ID
		WHERE t.Location IN (SELECT ID FROM scope_locs) AND m.Kind = 'hardware_resource'
		GROUP BY m.What
		ORDER BY cnt DESC
		LIMIT `+strconv.Itoa(limit), scope, lo, hi)
}

func (s *Server) httpTopBlockingResources(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	scope := r.FormValue("scope")

	limit := 10
	if v := r.FormValue("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	writeJSON(w, s.traceReader.TopBlockingResources(r.Context(), scope, limit))
}
