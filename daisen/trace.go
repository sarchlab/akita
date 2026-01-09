package main

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

	"github.com/sarchlab/akita/v4/sim"
)

func httpTrace(w http.ResponseWriter, r *http.Request) {
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

	// Parse pagination parameters
	limit, _ := strconv.Atoi(r.FormValue("limit"))
	offset, _ := strconv.Atoi(r.FormValue("offset"))

	// Apply default and max limits
	if limit <= 0 {
		limit = 1000 // default limit
	}
	if limit > 10000 {
		limit = 10000 // max limit
	}

	query := TaskQuery{
		ID:               r.FormValue("id"),
		ParentID:         r.FormValue("parentid"),
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
		Limit:            limit,
		Offset:           offset,
	}

	tasks, totalCount := traceReader.ListTasksPaginated(r.Context(), query)

	rsp := PaginatedTaskResponse{
		Data:       tasks,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
		HasMore:    offset+len(tasks) < totalCount,
	}

	rspBytes, err := json.Marshal(rsp)
	dieOnErr(err)

	_, err = w.Write(rspBytes)
	dieOnErr(err)
}

// TaskQuery is used to define the tasks to be queried. Not all the field has to
// be set. If the fields are empty, the criteria is ignored.
type TaskQuery struct {
	// Use ID to select a single task by its ID.
	ID string

	// Use ParentID to select all the tasks that are children of a task.
	ParentID string

	// Use Kind to select all the tasks that are of a kind.
	Kind string

	// Use Where to select all the tasks that are executed at a location.
	Where string

	// Enable time range selection.
	EnableTimeRange bool

	// Use StartTime to select tasks that overlaps with the given task range.
	StartTime, EndTime float64

	// EnableParentTask will also query the parent task of the selected tasks.
	EnableParentTask bool

	// Limit is the maximum number of results to return (default: 1000, max: 10000)
	Limit int

	// Offset is the number of results to skip (for pagination)
	Offset int
}

// PaginatedTaskResponse wraps task results with pagination metadata
type PaginatedTaskResponse struct {
	Data       []Task `json:"data"`
	TotalCount int    `json:"total_count"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
}

// A Task is a task
type TaskStep struct {
	Time sim.VTimeInSec `json:"time"`
	What string         `json:"what"`
	Kind string         `json:"kind"`
}

type Task struct {
	ID        string         `json:"id"`
	ParentID  string         `json:"parent_id"`
	Kind      string         `json:"kind"`
	What      string         `json:"what"`
	Location  string         `json:"location"`
	StartTime sim.VTimeInSec `json:"start_time"`
	EndTime   sim.VTimeInSec `json:"end_time"`
	Steps      []TaskStep     `json:"steps"`
	Detail     interface{} `json:"-"`
	ParentTask *Task       `json:"-"`
}

// TraceReader can parse a trace file.
type TraceReader interface {
	// ListComponents returns all the locations used in the trace.
	ListComponents(ctx context.Context) []string

	// ListTasks queries tasks.
	ListTasks(ctx context.Context, query TaskQuery) []Task

	// ListTasksPaginated queries tasks with pagination support.
	// Returns the tasks and the total count of matching tasks.
	ListTasksPaginated(ctx context.Context, query TaskQuery) ([]Task, int)
}

// SQLiteTraceReader is a reader that reads trace data from a SQLite database.
type SQLiteTraceReader struct {
	*sql.DB

	filename string

	// locationCache maps location IDs to location strings.
	// This is loaded from the "location" table which stores unique location
	// strings with integer IDs for efficient storage.
	locationCache map[int]string

	// locationReverseCache maps location strings to their IDs for query filtering.
	locationReverseCache map[string]int
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

	r.DB = db
	r.loadLocationCache()
}

// loadLocationCache loads the location ID to string mapping from the location table.
func (r *SQLiteTraceReader) loadLocationCache() {
	r.locationCache = make(map[int]string)
	r.locationReverseCache = make(map[string]int)

	rows, err := r.Query("SELECT ID, Locale FROM location")
	if err != nil {
		// Location table doesn't exist (old database format), fall back to legacy mode
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var locale string
		if err := rows.Scan(&id, &locale); err != nil {
			panic(err)
		}
		r.locationCache[id] = locale
		r.locationReverseCache[locale] = id
	}
}

// resolveLocation converts a location ID to its string representation.
// For legacy databases without the location table, it returns the input as-is.
func (r *SQLiteTraceReader) resolveLocation(locationIDStr string) string {
	if len(r.locationCache) == 0 {
		// Legacy mode: Location is stored as string directly
		return locationIDStr
	}

	id, err := strconv.Atoi(locationIDStr)
	if err != nil {
		// If it's not a number, it might be a legacy string location
		return locationIDStr
	}

	if loc, ok := r.locationCache[id]; ok {
		return loc
	}

	return locationIDStr
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
// If the location table exists (new format), it reads directly from there.
// Otherwise, it falls back to SELECT DISTINCT on the trace table (legacy format).
func (r *SQLiteTraceReader) ListComponents(ctx context.Context) []string {
	var components []string

	// If we have a location cache, use it directly (much faster)
	if len(r.locationCache) > 0 {
		for _, loc := range r.locationCache {
			components = append(components, loc)
		}
	} else {
		// Legacy mode: query distinct locations from trace table
		rows, err := r.QueryContext(ctx, "SELECT DISTINCT Location FROM trace")
		if err != nil {
			panic(err)
		}

		defer func() {
			err := rows.Close()
			if err != nil {
				panic(err)
			}
		}()

		for rows.Next() {
			var component string

			err := rows.Scan(&component)
			if err != nil {
				panic(err)
			}

			components = append(components, component)
		}
	}

	sort.Slice(components, func(i, j int) bool {
		return naturalLess(components[i], components[j])
	})

	return components
}

// ListTasks returns a list of tasks in the trace according to the given query.
func (r *SQLiteTraceReader) ListTasks(ctx context.Context, query TaskQuery) []Task {
	sqlStr := r.prepareTaskQueryStr(query)

	rows, err := r.QueryContext(ctx, sqlStr)
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	tasks := []Task{}

	for rows.Next() {
		task := r.scanTaskFromRow(rows, query.EnableParentTask)
		tasks = append(tasks, task)
	}

	// Always load milestones for tasks
	r.loadMilestonesForTasks(tasks)

	return tasks
}

// ListTasksPaginated returns a list of tasks with pagination support.
// It returns both the paginated tasks and the total count of matching tasks.
func (r *SQLiteTraceReader) ListTasksPaginated(
	ctx context.Context,
	query TaskQuery,
) ([]Task, int) {
	// Get total count first
	countSQL := r.prepareTaskCountStr(query)
	var totalCount int
	err := r.QueryRowContext(ctx, countSQL).Scan(&totalCount)
	if err != nil {
		panic(err)
	}

	// Get paginated tasks
	sqlStr := r.prepareTaskQueryStrPaginated(query)

	rows, err := r.QueryContext(ctx, sqlStr)
	if err != nil {
		panic(err)
	}

	defer rows.Close()

	tasks := []Task{}

	for rows.Next() {
		task := r.scanTaskFromRow(rows, query.EnableParentTask)
		tasks = append(tasks, task)
	}

	// Always load milestones for tasks
	r.loadMilestonesForTasks(tasks)

	return tasks, totalCount
}

// loadMilestonesForTasks loads milestones for the given tasks from the database
func (r *SQLiteTraceReader) loadMilestonesForTasks(tasks []Task) {
	if len(tasks) == 0 {
		return
	}

	// Build a map for quick task lookup
	taskMap := make(map[string]*Task)
	taskIDs := make([]interface{}, 0, len(tasks))
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
		taskIDs = append(taskIDs, tasks[i].ID)
	}

	// Query milestones for all tasks using parameterized query
	placeholders := strings.Repeat("?,", len(taskIDs))
	if len(placeholders) > 0 {
		placeholders = placeholders[:len(placeholders)-1] // remove trailing comma
	}
	sqlStr := fmt.Sprintf(`
		SELECT TaskID, Time, Kind, What, Location 
		FROM trace_milestones 
		WHERE TaskID IN (%s)
		ORDER BY TaskID, Time`, placeholders)

	rows, err := r.Query(sqlStr, taskIDs...)
	if err != nil {
		// If trace_milestones table doesn't exist, just return without error
		return
	}
	defer rows.Close()

	for rows.Next() {
		var taskID, kind, what, location string
		var time float64

		err := rows.Scan(&taskID, &time, &kind, &what, &location)
		if err != nil {
			continue
		}

		if task, exists := taskMap[taskID]; exists {
			step := TaskStep{
				Time:     sim.VTimeInSec(time),
				What:     what,
				Kind:     kind,
			}
			task.Steps = append(task.Steps, step)
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
	var ptID, ptParentID, ptKind, ptWhat, ptLocation sql.NullString
	var locationRaw string

	var ptStartTime, ptEndTime sql.NullFloat64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&locationRaw,
		&t.StartTime,
		&t.EndTime,
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

	t.Location = r.resolveLocation(locationRaw)

	if ptID.Valid {
		t.ParentTask.ID = ptID.String
		t.ParentTask.ParentID = ptParentID.String
		t.ParentTask.Kind = ptKind.String
		t.ParentTask.What = ptWhat.String
		t.ParentTask.Location = r.resolveLocation(ptLocation.String)
		t.ParentTask.StartTime = sim.VTimeInSec(ptStartTime.Float64)
		t.ParentTask.EndTime = sim.VTimeInSec(ptEndTime.Float64)
	}
}

func (r *SQLiteTraceReader) scanTaskWithoutParent(rows *sql.Rows, t *Task) {
	var locationRaw string
	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&locationRaw,
		&t.StartTime,
		&t.EndTime,
	)
	if err != nil {
		panic(err)
	}

	t.Location = r.resolveLocation(locationRaw)
}

func (r *SQLiteTraceReader) prepareTaskQueryStr(query TaskQuery) string {
	sqlStr := `
		SELECT 
			t.ID, 
			t.ParentID,
			t.Kind,
			t.What,
			t.Location,
			t.StartTime,
			t.EndTime
	`

	if query.EnableParentTask {
		sqlStr += `,
			pt.ID,
			pt.ParentID,
			pt.Kind,
			pt.What,
			pt.Location,
			pt.StartTime,
			pt.EndTime
		`
	}

	sqlStr += `
		FROM trace t
	`

	if query.EnableParentTask {
		sqlStr += `
			LEFT JOIN trace pt
			ON t.ParentID = pt.ID
		`
	}

	sqlStr = r.addQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

func (r *SQLiteTraceReader) addQueryConditionsToQueryStr(
	sqlStr string,
	query TaskQuery,
) string {
	sqlStr += `
		WHERE 1=1
	`

	if query.ID != "" {
		sqlStr += `
			AND t.ID = '` + query.ID + `'
		`
	}

	if query.ParentID != "" {
		sqlStr += `
			AND t.ParentID = '` + query.ParentID + `'
		`
	}

	if query.Kind != "" {
		sqlStr += `
			AND t.Kind = '` + query.Kind + `'
		`
	}

	if query.Where != "" {
		// If using location table (new format), look up the location ID
		if len(r.locationReverseCache) > 0 {
			if locID, ok := r.locationReverseCache[query.Where]; ok {
				sqlStr += fmt.Sprintf(`
			AND t.Location = %d
		`, locID)
			} else {
				// Location not found, add impossible condition to return empty results
				sqlStr += `
			AND 1=0
		`
			}
		} else {
			// Legacy mode: query by string directly
			sqlStr += `
			AND t.Location = '` + query.Where + `'
		`
		}
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND t.EndTime > %.15f AND t.StartTime < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}

// prepareTaskCountStr creates a SQL query to count matching tasks
func (r *SQLiteTraceReader) prepareTaskCountStr(query TaskQuery) string {
	sqlStr := `SELECT COUNT(*) FROM trace t`

	if query.EnableParentTask {
		sqlStr += `
			LEFT JOIN trace pt
			ON t.ParentID = pt.ID
		`
	}

	sqlStr = r.addQueryConditionsToQueryStr(sqlStr, query)

	return sqlStr
}

// prepareTaskQueryStrPaginated creates a SQL query with ORDER BY, LIMIT, and OFFSET
func (r *SQLiteTraceReader) prepareTaskQueryStrPaginated(query TaskQuery) string {
	sqlStr := r.prepareTaskQueryStr(query)

	// Add ORDER BY for consistent pagination results
	sqlStr += ` ORDER BY t.StartTime ASC, t.ID ASC`

	// Add LIMIT and OFFSET
	if query.Limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		sqlStr += fmt.Sprintf(" OFFSET %d", query.Offset)
	}

	return sqlStr
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

func httpSegments(w http.ResponseWriter, r *http.Request) {
	segments := traceReader.ListSegments(r.Context())

	rsp, err := json.Marshal(segments)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}
