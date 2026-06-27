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
	// Sample is the 1-in-N task stride used to compute this response. 1 is exact;
	// a larger value is an approximation (each sampled task's count scaled by
	// Sample) so a coarse preview returns fast and the client can refine by
	// re-requesting with a smaller Sample.
	Sample int `json:"sample"`
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

// exactScanTaskCap bounds an exact (sample=1) occupancy scan. Above this many
// tasks the event fan-out and GROUP BY cost minutes, so an exact request for a
// scope this large is declined — the true count is returned and the bins are left
// empty, so the caller keeps its sampled view. The client only asks for exact
// when its sampled estimate is small, so this trips solely when a dense scope was
// deterministically missed by the rowid sample.
const exactScanTaskCap = 200_000

// countTasksInScope counts the tasks overlapping [start, end) in a location scope
// via a plain index-only COUNT (no event fan-out, no GROUP BY), so it is far
// cheaper than the occupancy aggregation and can guard it. The bool is false if
// the count could not be run.
func (r *SQLiteTraceReader) countTasksInScope(
	ctx context.Context, scope string, start, end float64,
) (int, bool) {
	s := strconv.FormatFloat(start, 'f', -1, 64)
	e := strconv.FormatFloat(end, 'f', -1, 64)
	sqlStr := `
		WITH scope_locs AS (
			SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?)
		)
		SELECT COUNT(*) FROM trace t
		WHERE t.Location IN (SELECT ID FROM scope_locs)
			AND t.EndTime > ` + s + ` AND t.StartTime < ` + e

	lo, hi := scopePrefixBounds(scope)
	var n int
	if err := r.QueryRowContext(ctx, sqlStr, scope, lo, hi).Scan(&n); err != nil {
		return 0, false
	}

	return n, true
}

func (r *SQLiteTraceReader) ComponentTimeline( //nolint:funlen // one cohesive occupancy-binning SQL pipeline
	ctx context.Context,
	scope string,
	start, end float64,
	numBins int,
	groupByKind bool,
	sample int,
) ComponentTimelineResponse {
	if sample < 1 {
		sample = 1
	}
	resp := ComponentTimelineResponse{
		StartTime: start,
		EndTime:   end,
		NumBins:   numBins,
		Sample:    sample,
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
	// The color key groups tasks either by kind alone or by the finer kind-what
	// pair, mirroring the client's taskColorKey so a band lines up with the tasks
	// it represents.
	keyExpr := "t.Kind || '-' || t.What"
	if groupByKind {
		keyExpr = "t.Kind"
	}
	be := newBinExpr(numBins, start, end)

	// Covering index over exactly the columns this query reads, so it runs as an
	// index-only scan over the scope's locations instead of joining and fetching
	// 76M trace rows. Built single-flight so it persists (see ensureIndex).
	r.ensureIndex(
		ctx,
		"Building index idx_trace_loc_time_kind_what",
		`CREATE INDEX IF NOT EXISTS idx_trace_loc_time_kind_what `+
			`ON trace(Location, StartTime, EndTime, Kind, What)`,
	)

	// Guard the exact scan: a dense scope can be deterministically missed by the
	// rowid sample, prompting the client to request an exact pass. A cheap COUNT
	// confirms the scope is genuinely small before paying for the fan-out; if it is
	// large, return the true count and leave the bins empty (caller keeps sampled).
	if sample == 1 {
		if n, ok := r.countTasksInScope(ctx, scope, start, end); ok && n > exactScanTaskCap {
			resp.Total = n
			return resp
		}
	}

	// A 1-in-N task sample (on rowid, which the index leaf carries) makes a coarse
	// preview return fast; each sampled task stands in for `sample` tasks, so its
	// occupancy contribution is scaled back up by the same factor.
	sampleFilter := ""
	if sample > 1 {
		sampleFilter = " AND (t.rowid % " + strconv.Itoa(sample) + ") = 0"
	}

	// Resolve the scope's location IDs first (the location table is tiny), then
	// filter trace by those IDs so the covering index drives the scan.
	sqlStr := `
		WITH scope_locs AS (
			SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?)
		),
		events AS (
			SELECT
				CASE WHEN d.delta = 1
					THEN MAX(0, MIN(` + be.numBins + ` - 1, ` + be.floorOf("t.StartTime") + `))
					ELSE ` + be.ceilOf("t.EndTime") + `
				END AS bin,
				` + keyExpr + ` AS k,
				d.delta AS delta
			FROM trace t
			CROSS JOIN (SELECT 1 AS delta UNION ALL SELECT -1 AS delta) d
			WHERE t.Location IN (SELECT ID FROM scope_locs)
				AND t.EndTime > ` + be.startStr + ` AND t.StartTime < ` + be.endStr + sampleFilter + `
		)
		SELECT bin, k, delta, COUNT(*) * ` + strconv.Itoa(sample) + ` AS c
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
func (r *SQLiteTraceReader) BlockingReasonOccupancy( //nolint:funlen // one cohesive occupancy-binning SQL pipeline
	ctx context.Context,
	scope string,
	start, end float64,
	numBins int,
	sample int,
) (keys []string, bins [][]int) {
	if numBins < 1 || end <= start {
		return []string{}, [][]int{}
	}
	if sample < 1 {
		sample = 1
	}

	be := newBinExpr(numBins, start, end)
	// A 1-in-N task sample (on rowid) shrinks the trace×milestone join so a coarse
	// preview returns fast; each sampled task stands in for `sample` tasks, so its
	// blocked-time counts are scaled back up by the same factor.
	sampleFilter := ""
	if sample > 1 {
		sampleFilter = " AND (t.rowid % " + strconv.Itoa(sample) + ") = 0"
	}

	// Covering index that also carries ID, so the trace side of the trace×milestone
	// join is index-only (no per-task table lookup just to read t.ID for the join).
	r.ensureIndex(
		ctx,
		"Building index idx_trace_loc_time_id",
		`CREATE INDEX IF NOT EXISTS idx_trace_loc_time_id `+
			`ON trace(Location, StartTime, EndTime, ID)`,
	)

	// Guard the exact scan (see ComponentTimeline): the trace×milestone join is
	// even costlier, so decline it for a scope the rowid sample only appeared to
	// empty because it is dense and modulo-skewed.
	if sample == 1 {
		if n, ok := r.countTasksInScope(ctx, scope, start, end); ok && n > exactScanTaskCap {
			return []string{}, [][]int{}
		}
	}

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
				AND t.EndTime > ` + be.startStr + ` AND t.StartTime < ` + be.endStr + sampleFilter + `
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
		SELECT bin, k, delta, COUNT(*) * ` + strconv.Itoa(sample) + ` AS c
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

	// group=kind colors by task kind alone; anything else (default) keeps the
	// finer kind-what grouping.
	groupByKind := r.FormValue("group") == "kind"

	// sample is a 1-in-N task stride for a fast, approximate preview (default 1 =
	// exact). The client requests a coarse sample first and refines downward.
	sample := 1
	if v := r.FormValue("sample"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			sample = n
		}
	}

	writeJSON(w, s.traceReader.ComponentTimeline(
		r.Context(), scope, start, end, numBins, groupByKind, sample))
}
