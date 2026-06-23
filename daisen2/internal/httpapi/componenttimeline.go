package httpapi

import (
	"context"
	"database/sql"
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
// binExpr builds the SQL fragments that map a timestamp column to its bin index.
type binExpr struct {
	numBins  string
	startStr string
	endStr   string
	// floorOf is the bin a timestamp falls in; ceilOf is one past it — used for the
	// -1 end event so a value on a bin boundary doesn't bleed into the next bin.
	floorOf func(col string) string
	ceilOf  func(col string) string
}

func newBinExpr(numBins int, start, end float64) binExpr {
	nb := strconv.Itoa(numBins)
	s := strconv.FormatFloat(start, 'f', -1, 64)
	e := strconv.FormatFloat(end, 'f', -1, 64)
	// posOf is the fractional bin position of a timestamp. SQLite's CAST truncates
	// toward zero (floor, since positions here are non-negative); it has no ceil()
	// without the math extension, so ceilOf is floor(x) plus 1 when x has a fraction.
	posOf := func(col string) string {
		return "((" + col + " - " + s + ") * " + nb + " / (" + e + " - " + s + "))"
	}
	return binExpr{
		numBins:  nb,
		startStr: s,
		endStr:   e,
		floorOf:  func(col string) string { return "CAST(" + posOf(col) + " AS INTEGER)" },
		ceilOf: func(col string) string {
			p := posOf(col)
			return "(CAST(" + p + " AS INTEGER) + (" + p + " > CAST(" + p + " AS INTEGER)))"
		},
	}
}

// accumulateBins consumes (bin, key, delta, count) rows — a +1 at each interval's
// start bin and a -1 just past its end bin — and prefix-sums them into a dense
// numBins-by-key occupancy matrix. Shared by the kind-what task count and the
// blocking-reason count so both use the identical binning method. total counts the
// +1 (start) events, i.e. the number of intervals.
func accumulateBins(rows *sql.Rows, numBins int) (keys []string, bins [][]int, total int) {
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
		if delta == 1 {
			total += count
		}
	}

	keys = make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

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

	bins = make([][]int, numBins)
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
	return keys, bins, total
}

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

	// Count tasks ACTIVE in each bin (occupancy): a task spans every bin between
	// its start and end, emitting +1 at its start bin and -1 just past its end bin
	// (ceiling, so a task ending on a bin boundary is not counted into the next
	// bin). The cross join fans each task into its two events in one table scan.
	be := newBinExpr(numBins, start, end)
	sqlStr := `
		WITH events AS (
			SELECT
				CASE WHEN d.delta = 1
					THEN MAX(0, MIN(` + be.numBins + ` - 1, ` + be.floorOf("t.StartTime") + `))
					ELSE ` + be.ceilOf("t.EndTime") + `
				END AS bin,
				t.Kind || '-' || t.What AS k,
				d.delta AS delta
			FROM trace t
			JOIN location loc ON t.Location = loc.ID
			CROSS JOIN (SELECT 1 AS delta UNION ALL SELECT -1 AS delta) d
			WHERE (loc.Locale = ? OR (loc.Locale >= ? AND loc.Locale < ?))
				AND t.EndTime > ` + be.startStr + ` AND t.StartTime < ` + be.endStr + `
		)
		SELECT bin, k, delta, COUNT(*) AS c
		FROM events
		GROUP BY bin, k, delta`

	lo, hi := scopePrefixBounds(scope)
	rows, err := r.QueryContext(ctx, sqlStr, scope, lo, hi)
	if err != nil {
		return resp
	}
	defer rows.Close()

	resp.Keys, resp.Bins, resp.Total = accumulateBins(rows, numBins)
	return resp
}

// BlockingReasonOccupancy bins, per blocking reason, how many of a component's
// tasks are blocked on that reason in each time bin — the same occupancy binning
// ComponentTimeline uses for task kinds, just grouped by milestone kind. Each
// milestone marks the release of a blocking condition, so the interval ending at
// it (from the previous milestone, or the task's start) is time spent blocked on
// that milestone's kind. Computed entirely in SQL, with no per-task materialization.
func (r *SQLiteTraceReader) BlockingReasonOccupancy(
	ctx context.Context,
	scope string,
	start, end float64,
	numBins int,
) (keys []string, bins [][]int) {
	if numBins < 1 || end <= start {
		return []string{}, [][]int{}
	}

	be := newBinExpr(numBins, start, end)
	// Resolve the scope's location IDs first (the location table is tiny), then pull
	// only the trace rows at those locations via idx_trace_Location and their
	// milestones by TaskID. ivals is MATERIALIZED so the window runs over just the
	// scope's milestones — without the hint SQLite scans every milestone in TaskID
	// order to satisfy the window's PARTITION and probes the trace table 13M times.
	sqlStr := `
		WITH scope_locs AS (
			SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?)
		),
		ivals AS MATERIALIZED (
			SELECT
				m.Kind AS k,
				COALESCE(
					LAG(m.Time) OVER (PARTITION BY m.TaskID ORDER BY m.Time),
					t.StartTime
				) AS lo,
				m.Time AS hi
			FROM trace t
			JOIN milestone m ON m.TaskID = t.ID
			WHERE t.Location IN (SELECT ID FROM scope_locs)
				AND t.EndTime > ` + be.startStr + ` AND t.StartTime < ` + be.endStr + `
		),
		events AS (
			SELECT
				CASE WHEN d.delta = 1
					THEN MAX(0, MIN(` + be.numBins + ` - 1, ` + be.floorOf("lo") + `))
					ELSE ` + be.ceilOf("hi") + `
				END AS bin,
				k,
				d.delta AS delta
			FROM ivals
			CROSS JOIN (SELECT 1 AS delta UNION ALL SELECT -1 AS delta) d
			WHERE hi > ` + be.startStr + ` AND lo < ` + be.endStr + `
		)
		SELECT bin, k, delta, COUNT(*) AS c
		FROM events
		GROUP BY bin, k, delta`

	lo, hi := scopePrefixBounds(scope)
	rows, err := r.QueryContext(ctx, sqlStr, scope, lo, hi)
	if err != nil {
		return []string{}, [][]int{}
	}
	defer rows.Close()

	keys, bins, _ = accumulateBins(rows, numBins)
	return keys, bins
}

func (s *Server) httpComponentTimeline(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	scope := r.FormValue("scope")
	if scope == "" {
		// Fall back to the legacy `where` param so an older client (or cached
		// bundle) that has not learned `scope` yet still gets a real summary
		// instead of zero tasks (which would defeat the level-of-detail guard).
		scope = r.FormValue("where")
	}
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
