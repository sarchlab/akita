package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sarchlab/akita/v5/timing"
)

func (s *Server) httpTrace(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	tasks := s.traceReader.ListTasks(r.Context(), buildTraceQuery(r))

	rsp, err := json.Marshal(tasks)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

// buildTraceQuery parses the /api/trace request parameters into a TaskQuery.
func buildTraceQuery(r *http.Request) TaskQuery {
	useTimeRange := r.FormValue("starttime") != "" && r.FormValue("endtime") != ""

	startTime := 0.0
	endTime := 0.0

	if useTimeRange {
		var err error

		startTime, err = strconv.ParseFloat(r.FormValue("starttime"), 64)
		if err != nil {
			panic(err)
		}

		endTime, err = strconv.ParseFloat(r.FormValue("endtime"), 64)
		if err != nil {
			panic(err)
		}
	}

	var queryID uint64
	if idStr := r.FormValue("id"); idStr != "" {
		queryID, _ = strconv.ParseUint(idStr, 10, 64)
	}
	var queryParentID uint64
	if pidStr := r.FormValue("parentid"); pidStr != "" {
		queryParentID, _ = strconv.ParseUint(pidStr, 10, 64)
	}
	queryParentIDs := parseIDList(r.FormValue("parentids"))

	// The startup range probe (useSimulationRange's /api/trace?kind=Simulation) is a
	// bare global-Kind query with NO time range; it only needs the matched task's
	// span, not its steps, so loading milestones/tags for it would needlessly build
	// the full (TaskID, Time) milestone+tag indexes at dashboard start. The Task
	// chart's kind-filter browse also sends a global Kind query, but WITH a time
	// range, and uses the results' steps — so the no-time-range guard keeps
	// milestones for it.
	rangeProbe := r.FormValue("kind") != "" && !useTimeRange && r.FormValue("scope") == "" &&
		r.FormValue("where") == "" && queryID == 0 && queryParentID == 0

	return TaskQuery{
		ID:               queryID,
		ParentID:         queryParentID,
		ParentIDs:        queryParentIDs,
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		Scope:            r.FormValue("scope"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
		EnableMilestones: !rangeProbe,
	}
}

// parseIDList parses a comma-separated list of non-zero unsigned ids (e.g. the
// `parentids` query param), skipping blanks and unparseable entries.
func parseIDList(s string) []uint64 {
	if s == "" {
		return nil
	}

	var ids []uint64
	for _, part := range strings.Split(s, ",") {
		if id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64); err == nil && id != 0 {
			ids = append(ids, id)
		}
	}

	return ids
}

// joinIDs renders ids as a comma-separated string for an SQL IN (…) list. The
// values are uint64, so there is nothing to escape.
func joinIDs(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(id, 10)
	}

	return strings.Join(parts, ",")
}

// TaskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type TaskQuery struct {
	// Use ID to select a single task by its ID.
	ID uint64

	// Use ParentID to select all the tasks that are children of a task.
	ParentID uint64

	// Use ParentIDs to select the children of any of several parents in one
	// query — used to load a whole subtree level at once.
	ParentIDs []uint64

	// Use Kind to select all the tasks that are of a kind.
	Kind string

	// Use Where to select all the tasks that are executed at a location
	// (exact match on the full location string).
	Where string

	// Use Scope to select all the tasks at a location subtree: the scope
	// itself plus every location nested under it. Location names are dotted,
	// so Scope="TLB" matches "TLB", "TLB.req_in", "TLB.Top.incoming", etc.
	// This drives the dashboard's click-a-component drill-down.
	Scope string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select tasks that overlaps with the given task range.
	StartTime, EndTime float64

	// EnableParentTask will also query the parent task of the selected tasks.
	EnableParentTask bool

	// EnableMilestones will also query milestones for the selected tasks.
	EnableMilestones bool
}

// TaskStep represents a milestone/step in a task.
type TaskStep struct {
	Time timing.VTimeInPicoSec `json:"time"`
	What string                `json:"what"`
	Kind string                `json:"kind"`
}

// Task represents a traced task.
type Task struct {
	ID         uint64                `json:"id"`
	ParentID   uint64                `json:"parent_id"`
	Kind       string                `json:"kind"`
	What       string                `json:"what"`
	Location   string                `json:"location"`
	StartTime  timing.VTimeInPicoSec `json:"start_time"`
	EndTime    timing.VTimeInPicoSec `json:"end_time"`
	Steps      []TaskStep            `json:"steps"`
	Detail     interface{}           `json:"-"`
	ParentTask *Task                 `json:"-"`
}

// TraceReader can parse a trace file.
type TraceReader interface {
	// ListComponents returns all the locations used in the trace.
	ListComponents(ctx context.Context) []string

	// ListTasks queries tasks .
	ListTasks(ctx context.Context, query TaskQuery) []Task
}

// TraceTimeRange is the full time span covered by the trace table.
type TraceTimeRange struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// SQLiteTraceReader is a reader that reads trace data from a SQLite database.
type SQLiteTraceReader struct {
	*sql.DB

	filename string

	// activity tracks in-flight DB operations (index builds, heavy queries) so the
	// frontend can show what the database is doing. dbInfo caches the (expensive)
	// schema-and-size overview computed from dbstat.
	activity *DBActivityTracker
	dbInfo   *dbInfoCache

	// indexMu serializes on-demand CREATE INDEX so concurrent requests don't
	// contend on the single SQLite writer (which made the build fail with
	// "database is locked" and never persist). See ensureIndex.
	indexMu sync.Mutex

	// builtIndexes records the names of indexes ensureIndex has confirmed exist
	// this session. Repeat calls hit this set and return without taking indexMu,
	// so a request whose index already exists is never blocked behind an unrelated
	// index build holding the lock.
	builtIndexes sync.Map
}

var indexNameRe = regexp.MustCompile(`(?i)CREATE\s+INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?(\w+)`)

// indexNameFromDDL extracts the index name from a CREATE INDEX statement, or ""
// if it cannot be parsed (in which case ensureIndex falls back to always running
// the DDL under the lock).
func indexNameFromDDL(ddl string) string {
	m := indexNameRe.FindStringSubmatch(ddl)
	if len(m) < 2 {
		return ""
	}

	return m[1]
}

// indexExists reports whether an index of the given name is already in the
// schema. It is a plain read, so in WAL mode it runs concurrently with another
// goroutine building a different index and never blocks on the write lock.
func (r *SQLiteTraceReader) indexExists(ctx context.Context, name string) bool {
	var one int
	err := r.QueryRowContext(ctx,
		`SELECT 1 FROM sqlite_master WHERE type = 'index' AND name = ? LIMIT 1`,
		name).Scan(&one)

	return err == nil
}

// ensureIndex builds an index exactly once, on demand. It detaches from the
// request's cancellation via context.WithoutCancel (so a client navigating away
// mid-build can't cancel and roll it back, while still inheriting the request
// context's values), and serializes real builds through indexMu (so the 8
// concurrent dashboard/component requests don't fight over the single SQLite
// writer and abandon the build).
//
// An index already known to exist — built earlier this session, or persisted
// from a prior run — short-circuits without taking indexMu, so a request whose
// index is present is never blocked behind an unrelated, minutes-long build.
// Only a genuinely new build drops the cached db_info overview, since that is
// the only case that changes the database's on-disk size.
func (r *SQLiteTraceReader) ensureIndex(ctx context.Context, activityLabel, ddl string) {
	name := indexNameFromDDL(ddl)
	if name != "" {
		if _, done := r.builtIndexes.Load(name); done {
			return
		}
		if r.indexExists(ctx, name) {
			r.builtIndexes.Store(name, struct{}{})
			return
		}
	}

	r.indexMu.Lock()
	defer r.indexMu.Unlock()

	// Re-check under the lock: another goroutine may have built it while we waited.
	if name != "" {
		if _, done := r.builtIndexes.Load(name); done {
			return
		}
	}

	id := r.activity.Begin("index", activityLabel, "covering index")
	_, _ = r.ExecContext(context.WithoutCancel(ctx), ddl)
	r.activity.End(id)

	if name != "" {
		r.builtIndexes.Store(name, struct{}{})
	}
	// The new index enlarged the trace file; drop the cached overview so the next
	// /api/db_info reports the new sizes instead of stale ones until restart.
	r.dbInfo.invalidate()
}

// NewSQLiteTraceReader creates a new SQLiteTraceReader.
func NewSQLiteTraceReader(filename string) *SQLiteTraceReader {
	r := &SQLiteTraceReader{
		filename: filename,
		activity: NewDBActivityTracker(),
		dbInfo:   &dbInfoCache{},
	}

	return r
}

// Init establishes a connection to the database.
func (r *SQLiteTraceReader) Init() {
	db, err := sql.Open("sqlite3", r.filename)
	if err != nil {
		panic(err)
	}

	// Enable WAL mode for concurrent read access.
	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		panic(err)
	}

	r.DB = db
}

// InitReadOnly establishes a read-only connection to the database. It is used to
// read a trace concurrently while a DBTracer writes it; the writer already puts
// the database in WAL mode, and a read-only connection must not set the journal
// mode itself. With the native driver "mode=ro" is a true read-only open, so
// setting WAL here would fail on any non-WAL file and panic — hence no such
// pragma below.
func (r *SQLiteTraceReader) InitReadOnly() {
	db, err := sql.Open("sqlite3", r.filename+"?mode=ro")
	if err != nil {
		panic(err)
	}

	r.DB = db
}

func naturalLess(a, b string) bool {
	re := regexp.MustCompile(`\d+|\D+`)
	as := re.FindAllString(a, -1)
	bs := re.FindAllString(b, -1)

	for i := 0; i < len(as) && i < len(bs); i++ {
		anum, aErr := strconv.Atoi(as[i])
		bnum, bErr := strconv.Atoi(bs[i])

		if aErr == nil && bErr == nil {
			if anum != bnum {
				return anum < bnum
			}
		} else {
			if as[i] != bs[i] {
				return as[i] < bs[i]
			}
		}
	}

	return len(as) < len(bs)
}

// ListComponents returns a list of components in the trace.
func (r *SQLiteTraceReader) ListComponents(ctx context.Context) []string {
	var components []string

	// The shared location table holds exactly the set of component names used
	// in the trace, each interned once.
	rows, err := r.QueryContext(ctx, "SELECT Locale FROM location")
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		panic(err)
	}

	defer func() {
		err := rows.Close()
		if err != nil && ctx.Err() == nil {
			panic(err)
		}
	}()

	for rows.Next() {
		var component string

		err := rows.Scan(&component)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			panic(err)
		}

		components = append(components, component)
	}

	sort.Slice(components, func(i, j int) bool {
		return naturalLess(components[i], components[j])
	})

	return components
}

// ListTasks returns a list of tasks in the trace according to the given query.
var (
	// safeKindPattern restricts the Kind values a partial index may be built for
	// to characters that appear in real task kinds, so the value can be embedded
	// as a SQL literal in the index DDL with no injection risk.
	safeKindPattern  = regexp.MustCompile(`^[A-Za-z0-9_ .\[\]\-]+$`)
	kindIdentReplace = strings.NewReplacer(" ", "_", ".", "_", "[", "_", "]", "_", "-", "_")
)

// ensureTaskQueryIndexes builds, on demand, the secondary indexes a given task
// query needs. The simulation writes the trace without any secondary indexes on
// trace/milestone/tag (it never queries the data — see the entry structs in
// akita's tracing.dbtracer), so the reader owns them and builds only what an
// access pattern actually uses, once per trace.
func (r *SQLiteTraceReader) ensureTaskQueryIndexes(ctx context.Context, query TaskQuery) {
	if query.ID != 0 {
		r.ensureIndex(ctx, "Building index idx_trace_ID",
			"CREATE INDEX IF NOT EXISTS idx_trace_ID ON trace(ID)")
	}
	if query.ParentID != 0 {
		r.ensureIndex(ctx, "Building index idx_trace_ParentID",
			"CREATE INDEX IF NOT EXISTS idx_trace_ParentID ON trace(ParentID)")
	}

	// Only the startup range probe — a global Kind lookup with NO time range — builds
	// a partial Kind index (tiny, on just that value). The Task chart's kind-filter
	// browse sends a global Kind query per keystroke WITH a time range; gating on
	// !EnableTimeRange avoids spamming a partial index for every typed prefix (that
	// browse scans instead, which is fine for an interactive, controlled filter).
	if query.Kind != "" && !query.EnableTimeRange && query.Scope == "" && query.Where == "" &&
		query.ID == 0 && query.ParentID == 0 && safeKindPattern.MatchString(query.Kind) {
		ident := kindIdentReplace.Replace(query.Kind)
		r.ensureIndex(ctx, "Building partial index idx_trace_kind_"+ident,
			fmt.Sprintf(
				"CREATE INDEX IF NOT EXISTS idx_trace_kind_%s ON trace(Kind) WHERE Kind = '%s'",
				ident, query.Kind))
	}
}

func (r *SQLiteTraceReader) ListTasks(ctx context.Context, query TaskQuery) []Task {
	r.ensureTaskQueryIndexes(ctx, query)

	sqlStr, args := r.prepareTaskQueryStr(query)

	rows, err := r.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	tasks := []Task{}

	for rows.Next() {
		task := r.scanTaskFromRow(rows, query.EnableParentTask)
		tasks = append(tasks, task)
	}

	if query.EnableMilestones {
		r.loadMilestonesForTasks(ctx, tasks)
		r.loadTagsForTasks(ctx, tasks)
		sortTaskSteps(tasks)
	}

	return tasks
}

// listTaskIntervals fetches only the [StartTime, EndTime) intervals of the tasks
// in a location scope that overlap [start, end), optionally restricted to a single
// Kind. It is the lean alternative to ListTasks for occupancy-style metrics that
// need nothing but the intervals — one (covering) index scan rather than hydrating
// every Task. The scope is the named location plus anything nested under it, so a
// component name aggregates its whole subtree while a leaf matches only itself. An
// empty kind matches every kind; empty whatLikes matches every What, otherwise the
// What must match at least one of the SQL LIKE patterns (e.g. "%Req", "%Request").
// Each returned Task has only StartTime and EndTime set.
func (r *SQLiteTraceReader) listTaskIntervals(
	ctx context.Context,
	location, kind string,
	whatLikes []string,
	start, end float64,
) []Task {
	q := `
		SELECT StartTime, EndTime
		FROM trace
		WHERE Location IN (SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?))
			AND EndTime > ? AND StartTime < ?`

	lo, hi := scopePrefixBounds(location)
	args := []any{location, lo, hi, start, end}
	if kind != "" {
		q += "\n\t\t\tAND Kind = ?"
		args = append(args, kind)
	}
	if len(whatLikes) > 0 {
		clauses := make([]string, len(whatLikes))
		for i, pattern := range whatLikes {
			clauses[i] = "What LIKE ?"
			args = append(args, pattern)
		}
		q += "\n\t\t\tAND (" + strings.Join(clauses, " OR ") + ")"
	}

	rows, err := r.QueryContext(ctx, q, args...)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		panic(err)
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var s, e float64
		if err := rows.Scan(&s, &e); err != nil {
			if ctx.Err() != nil {
				return tasks
			}
			panic(err)
		}
		tasks = append(tasks, Task{
			StartTime: timing.VTimeInPicoSec(s),
			EndTime:   timing.VTimeInPicoSec(e),
		})
	}

	return tasks
}

// sortTaskSteps orders each task's Steps by time, so milestones and tags loaded
// from separate tables form one coherent timeline.
func sortTaskSteps(tasks []Task) {
	for i := range tasks {
		steps := tasks[i].Steps
		sort.SliceStable(steps, func(a, b int) bool {
			return steps[a].Time < steps[b].Time
		})
	}
}

// TimeRange returns the min task start time and max task end time in the trace.
func (r *SQLiteTraceReader) TimeRange(ctx context.Context) (TraceTimeRange, bool) {
	if timeRange, ok := r.execInfoTimeRange(ctx); ok {
		return timeRange, true
	}

	row := r.QueryRowContext(ctx, "SELECT MIN(StartTime), MAX(EndTime) FROM trace")

	var startTime, endTime sql.NullFloat64
	err := row.Scan(&startTime, &endTime)
	if err != nil {
		if ctx.Err() != nil {
			return TraceTimeRange{}, false
		}
		panic(err)
	}

	if !startTime.Valid || !endTime.Valid || startTime.Float64 >= endTime.Float64 {
		return TraceTimeRange{}, false
	}

	return TraceTimeRange{
		StartTime: startTime.Float64,
		EndTime:   endTime.Float64,
	}, true
}

func (r *SQLiteTraceReader) execInfoTimeRange(ctx context.Context) (TraceTimeRange, bool) {
	rows, err := r.QueryContext(ctx, `
		SELECT Property, Value
		FROM exec_info
		WHERE Property IN ('Start Virtual Time', 'End Virtual Time')
	`)
	if err != nil {
		return TraceTimeRange{}, false
	}
	defer rows.Close()

	var timeRange TraceTimeRange
	var hasStart, hasEnd bool
	for rows.Next() {
		var property, value string
		err := rows.Scan(&property, &value)
		if err != nil {
			return TraceTimeRange{}, false
		}

		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return TraceTimeRange{}, false
		}

		switch property {
		case "Start Virtual Time":
			timeRange.StartTime = parsed
			hasStart = true
		case "End Virtual Time":
			timeRange.EndTime = parsed
			hasEnd = true
		}
	}

	if !hasStart || !hasEnd || timeRange.StartTime >= timeRange.EndTime {
		return TraceTimeRange{}, false
	}

	return timeRange, true
}

func (s *Server) httpTraceTimeRange(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	timeRange, ok := s.traceReader.TimeRange(r.Context())
	if !ok {
		http.Error(w, "trace time range not available", http.StatusNotFound)
		return
	}

	rsp, err := json.Marshal(timeRange)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

// loadMilestonesForTasks loads milestones for the given tasks from the database.
func (r *SQLiteTraceReader) loadMilestonesForTasks(ctx context.Context, tasks []Task) {
	if len(tasks) == 0 {
		return
	}

	// Built by the reader (the sim no longer indexes milestones). A composite
	// (TaskID, Time) serves both the TaskID IN-filter and the ORDER BY TaskID,Time.
	r.ensureIndex(ctx, "Building index idx_milestone_TaskID_Time",
		"CREATE INDEX IF NOT EXISTS idx_milestone_TaskID_Time ON milestone(TaskID, Time)")

	// Build a map for quick task lookup
	taskMap := make(map[uint64]*Task)
	taskIDs := make([]uint64, 0, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
		taskIDs = append(taskIDs, tasks[i].ID)
	}

	// SQLite caps the number of bound parameters in one statement
	// (SQLITE_MAX_VARIABLE_NUMBER, ~32k). A component can have far more tasks than
	// that, so query in batches — a single IN list with one placeholder per task
	// would make the statement fail and silently drop every milestone for big
	// components (which left the blocking-reason chart blank when zoomed out).
	const batchSize = 10000
	for start := 0; start < len(taskIDs); start += batchSize {
		end := min(start+batchSize, len(taskIDs))
		r.loadMilestoneBatch(taskMap, taskIDs[start:end])
	}
}

// loadMilestoneBatch loads milestones for one batch of task ids into taskMap.
func (r *SQLiteTraceReader) loadMilestoneBatch(taskMap map[uint64]*Task, ids []uint64) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	// A milestone's location is inherited from its task, so the milestone
	// table no longer stores it; we read the remaining columns only.
	sqlStr := fmt.Sprintf(`
		SELECT TaskID, Time, Kind, What
		FROM milestone
		WHERE TaskID IN (%s)
		ORDER BY TaskID, Time`, placeholders)

	rows, err := r.Query(sqlStr, args...)
	if err != nil {
		// If milestone table doesn't exist, just return without error
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID uint64
		var kind, what string
		var time float64

		err := rows.Scan(&taskID, &time, &kind, &what)
		if err != nil {
			continue
		}

		if task, exists := taskMap[taskID]; exists {
			step := TaskStep{
				Time: timing.VTimeInPicoSec(uint64(time)),
				What: what,
				Kind: kind,
			}
			task.Steps = append(task.Steps, step)
		}
	}
}

// loadTagsForTasks loads the categorical tags persisted in the tag table for
// the given tasks and merges them into each task's Steps stream alongside
// milestones. A tag's location is inherited from its task, so the tag table
// stores none; tags also carry no Kind, so they are labelled "tag" to stay
// distinguishable from milestones in the merged stream.
func (r *SQLiteTraceReader) loadTagsForTasks(ctx context.Context, tasks []Task) {
	if len(tasks) == 0 {
		return
	}

	// Built by the reader (the sim no longer indexes tags).
	r.ensureIndex(ctx, "Building index idx_tag_TaskID_Time",
		"CREATE INDEX IF NOT EXISTS idx_tag_TaskID_Time ON tag(TaskID, Time)")

	taskMap := make(map[uint64]*Task)
	taskIDs := make([]uint64, 0, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
		taskIDs = append(taskIDs, tasks[i].ID)
	}

	// Batch like loadMilestonesForTasks: one placeholder per task overflows
	// SQLite's bound-parameter limit for large components.
	const batchSize = 10000
	for start := 0; start < len(taskIDs); start += batchSize {
		end := min(start+batchSize, len(taskIDs))
		r.loadTagBatch(taskMap, taskIDs[start:end])
	}
}

// loadTagBatch loads tags for one batch of task ids into taskMap.
func (r *SQLiteTraceReader) loadTagBatch(taskMap map[uint64]*Task, ids []uint64) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	sqlStr := fmt.Sprintf(`
		SELECT TaskID, Time, What
		FROM tag
		WHERE TaskID IN (%s)
		ORDER BY TaskID, Time`, placeholders)

	rows, err := r.Query(sqlStr, args...)
	if err != nil {
		// If the tag table doesn't exist, just return without error.
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID uint64
		var what string
		var time float64

		err := rows.Scan(&taskID, &time, &what)
		if err != nil {
			continue
		}

		if task, exists := taskMap[taskID]; exists {
			task.Steps = append(task.Steps, TaskStep{
				Time: timing.VTimeInPicoSec(uint64(time)),
				What: what,
				Kind: "tag",
			})
		}
	}
}

func (r *SQLiteTraceReader) scanTaskFromRow(
	rows *sql.Rows,
	enableParentTask bool,
) Task {
	t := Task{}

	if enableParentTask {
		t.ParentTask = &Task{}
		r.scanTaskWithParent(rows, &t)
	} else {
		r.scanTaskWithoutParent(rows, &t)
	}

	return t
}

func (r *SQLiteTraceReader) scanTaskWithParent(rows *sql.Rows, t *Task) {
	var ptID, ptParentID sql.NullInt64
	var ptKind, ptWhat, ptLocation sql.NullString
	var ptStartTime, ptEndTime sql.NullFloat64
	var startTime, endTime float64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
		&startTime,
		&endTime,
		&ptID,
		&ptParentID,
		&ptKind,
		&ptWhat,
		&ptLocation,
		&ptStartTime,
		&ptEndTime,
	)
	if err != nil {
		panic(err)
	}

	t.StartTime = timing.VTimeInPicoSec(uint64(startTime))
	t.EndTime = timing.VTimeInPicoSec(uint64(endTime))

	if ptID.Valid {
		t.ParentTask.ID = uint64(ptID.Int64)
		t.ParentTask.ParentID = uint64(ptParentID.Int64)
		t.ParentTask.Kind = ptKind.String
		t.ParentTask.What = ptWhat.String
		t.ParentTask.Location = ptLocation.String
		t.ParentTask.StartTime = timing.VTimeInPicoSec(uint64(ptStartTime.Float64))
		t.ParentTask.EndTime = timing.VTimeInPicoSec(uint64(ptEndTime.Float64))
	}
}

func (r *SQLiteTraceReader) scanTaskWithoutParent(rows *sql.Rows, t *Task) {
	var startTime, endTime float64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
		&startTime,
		&endTime,
	)
	if err != nil {
		panic(err)
	}

	t.StartTime = timing.VTimeInPicoSec(uint64(startTime))
	t.EndTime = timing.VTimeInPicoSec(uint64(endTime))
}

func (r *SQLiteTraceReader) prepareTaskQueryStr(query TaskQuery) (string, []any) {
	// Location is stored as an integer id that references the shared location
	// table; join it back to the component name string.
	sqlStr := `
		SELECT
			t.ID,
			t.ParentID,
			t.Kind,
			t.What,
			loc.Locale,
			t.StartTime,
			t.EndTime
	`

	if query.EnableParentTask {
		sqlStr += `,
			pt.ID,
			pt.ParentID,
			pt.Kind,
			pt.What,
			ploc.Locale,
			pt.StartTime,
			pt.EndTime
		`
	}

	sqlStr += `
		FROM trace t
		JOIN location loc ON t.Location = loc.ID
	`

	if query.EnableParentTask {
		sqlStr += `
			LEFT JOIN trace pt
			ON t.ParentID = pt.ID
			LEFT JOIN location ploc
			ON pt.Location = ploc.ID
		`
	}

	sqlStr, args := r.addQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr, args
}

func (*SQLiteTraceReader) addQueryConditionsToQueryStr(
	sqlStr string,
	query TaskQuery,
) (string, []any) {
	args := []any{}

	sqlStr += `
		WHERE 1=1
	`

	if query.ID != 0 {
		sqlStr += `
			AND t.ID = ` + strconv.FormatUint(query.ID, 10) + `
		`
	}

	if query.ParentID != 0 {
		sqlStr += `
			AND t.ParentID = ` + strconv.FormatUint(query.ParentID, 10) + `
		`
	}

	if len(query.ParentIDs) > 0 {
		sqlStr += `
			AND t.ParentID IN (` + joinIDs(query.ParentIDs) + `)
		`
	}

	if query.Kind != "" {
		sqlStr += `
			AND t.Kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		// Resolve the location id first, then probe `trace` by its Location index
		// (see the Scope branch for why the joined-Locale filter is avoided).
		// Parameterized, which also drops the value out of the SQL text.
		sqlStr += `
			AND t.Location IN (SELECT ID FROM location WHERE Locale = ?)
		`
		args = append(args, query.Where)
	}

	if query.Scope != "" {
		// Select the scope component and everything nested under it. Locations
		// are dotted, so the subtree is the exact name plus the "scope." prefix.
		// A case-sensitive range ([scope+".", scope+"/")) is used instead of LIKE
		// because SQLite's LIKE is ASCII case-insensitive while the location tree
		// and the exact `=` check are case-sensitive — LIKE would pull in a
		// differently-cased sibling subtree.
		//
		// Filter by the resolved location-id set (a sub-select over the small
		// location table) rather than joining and filtering on loc.Locale: that
		// lets SQLite resolve the handful of in-scope location ids first and probe
		// `trace` by its (Location, StartTime, EndTime) index. The join form let
		// the planner drive from a bare time-range index instead, scanning every
		// task after StartTime — turning a 107-row leaf-scope query into a ~25s
		// near-full scan. The outer JOIN stays only to fetch each task's Locale.
		lo, hi := scopePrefixBounds(query.Scope)
		sqlStr += `
			AND t.Location IN (
				SELECT ID FROM location
				WHERE Locale = ? OR (Locale >= ? AND Locale < ?)
			)
		`
		args = append(args, query.Scope, lo, hi)
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND t.EndTime > %.15f AND t.StartTime < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr, args
}

// scopePrefixBounds returns the half-open [lo, hi) Locale range that selects
// every location nested under scope ("scope." followed by anything), in SQLite's
// case-sensitive BINARY ordering. "/" (0x2F) is the byte right after "." (0x2E),
// so every "scope." string sorts below it and no other prefix slips in. Pair it
// with an exact `Locale = scope` check to also include the scope's own location.
func scopePrefixBounds(scope string) (lo, hi string) {
	return scope + ".", scope + "/"
}

// Segment represents a time segment where traces were collected
type Segment struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// SegmentsResponse contains the segments data and whether the feature is enabled
type SegmentsResponse struct {
	Enabled  bool      `json:"enabled"`
	Segments []Segment `json:"segments"`
}

// HasSegmentsTable checks if the daisen$segments table exists in the database
func (r *SQLiteTraceReader) HasSegmentsTable(ctx context.Context) bool {
	query := `SELECT name FROM sqlite_master WHERE type='table' AND name='daisen$segments'`
	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return false
	}
	defer rows.Close()

	return rows.Next()
}

// ListSegments returns all segments from the daisen$segments table
func (r *SQLiteTraceReader) ListSegments(ctx context.Context) SegmentsResponse {
	response := SegmentsResponse{
		Enabled:  false,
		Segments: []Segment{},
	}

	if !r.HasSegmentsTable(ctx) {
		return response
	}

	response.Enabled = true

	query := `SELECT StartTime, EndTime FROM "daisen$segments" ORDER BY StartTime`
	rows, err := r.QueryContext(ctx, query)
	if err != nil {
		return response
	}
	defer rows.Close()

	for rows.Next() {
		var segment Segment
		err := rows.Scan(&segment.StartTime, &segment.EndTime)
		if err != nil {
			continue
		}
		response.Segments = append(response.Segments, segment)
	}

	return response
}

func (s *Server) httpSegments(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	segments := s.traceReader.ListSegments(r.Context())

	rsp, err := json.Marshal(segments)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}
