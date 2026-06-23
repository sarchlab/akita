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

	_ "github.com/mattn/go-sqlite3"
	"github.com/sarchlab/akita/v5/timing"
)

func (s *Server) httpTrace(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	useTimeRange := true
	if r.FormValue("starttime") == "" || r.FormValue("endtime") == "" {
		useTimeRange = false
	}

	var err error

	startTime := 0.0
	endTime := 0.0

	if useTimeRange {
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

	query := TaskQuery{
		ID:               queryID,
		ParentID:         queryParentID,
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		Scope:            r.FormValue("scope"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
		EnableMilestones: true,
	}

	tasks := s.traceReader.ListTasks(r.Context(), query)

	rsp, err := json.Marshal(tasks)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

// TaskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type TaskQuery struct {
	// Use ID to select a single task by its ID.
	ID uint64

	// Use ParentID to select all the tasks that are children of a task.
	ParentID uint64

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
}

// NewSQLiteTraceReader creates a new SQLiteTraceReader.
func NewSQLiteTraceReader(filename string) *SQLiteTraceReader {
	r := &SQLiteTraceReader{
		filename: filename,
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
func (r *SQLiteTraceReader) ListTasks(ctx context.Context, query TaskQuery) []Task {
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
		r.loadMilestonesForTasks(tasks)
		r.loadTagsForTasks(tasks)
		sortTaskSteps(tasks)
	}

	return tasks
}

// listTaskIntervals fetches only the [StartTime, EndTime) intervals of the tasks
// in a location scope that overlap [start, end). It is the lean alternative to
// ListTasks for occupancy-style metrics that need nothing but the intervals — one
// (covering) index scan rather than hydrating every Task. The scope is the named
// location plus anything nested under it, so a component name aggregates its whole
// subtree while a leaf matches only itself. Each returned Task has only StartTime
// and EndTime set.
func (r *SQLiteTraceReader) listTaskIntervals(
	ctx context.Context,
	location string,
	start, end float64,
) []Task {
	const q = `
		SELECT StartTime, EndTime
		FROM trace
		WHERE Location IN (SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?))
			AND EndTime > ? AND StartTime < ?`

	lo, hi := scopePrefixBounds(location)
	rows, err := r.QueryContext(ctx, q, location, lo, hi, start, end)
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
func (r *SQLiteTraceReader) loadMilestonesForTasks(tasks []Task) {
	if len(tasks) == 0 {
		return
	}

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
func (r *SQLiteTraceReader) loadTagsForTasks(tasks []Task) {
	if len(tasks) == 0 {
		return
	}

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

	if query.Kind != "" {
		sqlStr += `
			AND t.Kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		sqlStr += `
			AND loc.Locale = '` + query.Where + `'
		`
	}

	if query.Scope != "" {
		// Select the scope component and everything nested under it. Locations
		// are dotted, so the subtree is the exact name plus the "scope." prefix.
		// Match the exact location or anything nested under it. A case-sensitive
		// range ([scope+".", scope+"/")) is used instead of LIKE because SQLite's
		// LIKE is ASCII case-insensitive while the location tree and the exact `=`
		// check are case-sensitive — LIKE would pull in a differently-cased sibling
		// subtree. Parameterized to keep the scope value out of the SQL text.
		lo, hi := scopePrefixBounds(query.Scope)
		sqlStr += `
			AND (loc.Locale = ? OR (loc.Locale >= ? AND loc.Locale < ?))
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
