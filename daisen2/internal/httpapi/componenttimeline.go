package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
)

// ComponentTimelineResponse is a downsampled, level-of-detail view of a
// component's tasks over a time range. Instead of returning every task (which
// can be hundreds of thousands for a busy component), it reports per-time-bin
// occupancy grouped by color key ("Kind-What"), so the client can draw a density
// chart with a per-kind breakdown (and still highlight a kind from the legend)
// without shipping or rendering one element per task.
type ComponentTimelineResponse struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	NumBins   int     `json:"num_bins"`
	// Total is the number of tasks overlapping the range — i.e. how many the
	// per-task view would have to render. The client uses it to decide between
	// this aggregated view and the raw per-task view.
	Total int `json:"total"`
	// Keys are the distinct "Kind-What" color keys present, sorted, matching the
	// column order of every Bins row.
	Keys []string `json:"keys"`
	// Bins is a dense NumBins-by-len(Keys) matrix of occupancy counts: how many
	// tasks of each key are active in each bin.
	Bins [][]int `json:"bins"`
}

// ComponentTimeline bins the tasks in a location scope over [start, end) into
// numBins columns, reporting per-bin occupancy (active task count) per
// "Kind-What" key. The scope is a location subtree — the exact location plus
// everything dotted beneath it — so an internal node (e.g. "ROB") aggregates all
// of "ROB.req_in", "ROB.Top.incoming", … while a leaf matches only itself. Total
// is the number of tasks overlapping the range — the same set the per-task view
// would draw — which the client uses to pick between the two views.
func (r *SQLiteTraceReader) ComponentTimeline(
	ctx context.Context,
	scope string,
	start, end float64,
	numBins int,
) ComponentTimelineResponse {
	resp := ComponentTimelineResponse{
		StartTime: start,
		EndTime:   end,
		NumBins:   numBins,
		Keys:      []string{},
		Bins:      [][]int{},
	}

	if numBins < 1 || end <= start {
		return resp
	}

	// Count tasks ACTIVE in each bin (occupancy), not just the ones that START in
	// it. A task spans every bin between its start and end, so a long-lived task
	// contributes to all of them. Counting only starts makes a component that
	// fires tasks in bursts look spiky (tall spikes at the bursts, empty valleys
	// in between) even while it stays busy with long, still-running tasks.
	//
	// Occupancy is computed as a difference array: emit +1 at the bin a task starts
	// in and -1 at the first bin after it ends, then prefix-sum. The end event uses
	// the ceiling of the end position, not floor+1, so a task whose EndTime lands
	// exactly on a bin boundary is not counted into the next bin (its interval is
	// half-open). The cross join fans each task into its two events in a single
	// table scan; -1 events past the last bin (tasks still running at range end)
	// are simply dropped.
	nb := strconv.Itoa(numBins)
	s := strconv.FormatFloat(start, 'f', -1, 64)
	e := strconv.FormatFloat(end, 'f', -1, 64)
	// posOf is the fractional bin position of a timestamp. SQLite's CAST truncates
	// toward zero (floor, since positions here are non-negative); it has no ceil()
	// without the math extension, so ceilOf is floor(x) plus 1 when x has a
	// fractional part.
	posOf := func(col string) string {
		return "((" + col + " - " + s + ") * " + nb + " / (" + e + " - " + s + "))"
	}
	floorOf := func(col string) string {
		return "CAST(" + posOf(col) + " AS INTEGER)"
	}
	ceilOf := func(col string) string {
		p := posOf(col)
		return "(CAST(" + p + " AS INTEGER) + (" + p + " > CAST(" + p + " AS INTEGER)))"
	}

	sqlStr := `
		WITH events AS (
			SELECT
				CASE WHEN d.delta = 1
					THEN MAX(0, MIN(` + nb + ` - 1, ` + floorOf("t.StartTime") + `))
					ELSE ` + ceilOf("t.EndTime") + `
				END AS bin,
				t.Kind || '-' || t.What AS k,
				d.delta AS delta
			FROM trace t
			JOIN location loc ON t.Location = loc.ID
			CROSS JOIN (SELECT 1 AS delta UNION ALL SELECT -1 AS delta) d
			WHERE (loc.Locale = ? OR loc.Locale LIKE ? ESCAPE '\')
				AND t.EndTime > ` + s + ` AND t.StartTime < ` + e + `
		)
		SELECT bin, k, delta, COUNT(*) AS c
		FROM events
		GROUP BY bin, k, delta`

	rows, err := r.QueryContext(ctx, sqlStr, scope, escapeLikePrefix(scope)+`.%`)
	if err != nil {
		return resp
	}
	defer rows.Close()

	type event struct {
		bin   int
		key   string
		delta int
		count int
	}
	events := []event{}
	keySet := map[string]struct{}{}

	for rows.Next() {
		var bin, delta, count int
		var key string
		if err := rows.Scan(&bin, &key, &delta, &count); err != nil {
			continue
		}
		events = append(events, event{bin, key, delta, count})
		keySet[key] = struct{}{}
		// Every task emits exactly one +1 (start) event, so the start events count
		// the tasks overlapping the range — what the per-task view would draw.
		if delta == 1 {
			resp.Total += count
		}
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	resp.Keys = keys

	keyIndex := make(map[string]int, len(keys))
	for i, k := range keys {
		keyIndex[k] = i
	}

	// diff[bin][key]; a -1 at bin numBins (or beyond) is out of range and ignored.
	diff := make([][]int, numBins+1)
	for i := range diff {
		diff[i] = make([]int, len(keys))
	}
	for _, ev := range events {
		if ev.bin < 0 || ev.bin >= numBins {
			continue
		}
		diff[ev.bin][keyIndex[ev.key]] += ev.delta * ev.count
	}

	bins := make([][]int, numBins)
	running := make([]int, len(keys))
	for b := range numBins {
		row := make([]int, len(keys))
		for ki := range keys {
			running[ki] += diff[b][ki]
			if running[ki] < 0 {
				running[ki] = 0
			}
			row[ki] = running[ki]
		}
		bins[b] = row
	}
	resp.Bins = bins

	return resp
}

func (s *Server) httpComponentTimeline(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	scope := r.FormValue("scope")
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
		if n, err := strconv.Atoi(v); err == nil {
			numBins = n
		}
	}
	if numBins < 1 {
		numBins = 1
	}
	if numBins > 2000 {
		numBins = 2000
	}

	writeJSON(w, s.traceReader.ComponentTimeline(r.Context(), scope, start, end, numBins))
}
