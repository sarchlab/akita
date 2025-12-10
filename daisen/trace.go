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
	fmt.Printf("httpTrace called with URL: %s\n", r.URL.String())
	fmt.Printf("Form values - starttime: '%s', endtime: '%s'\n", r.FormValue("starttime"), r.FormValue("endtime"))
	
	useTimeRange := true
	if r.FormValue("starttime") == "" || r.FormValue("endtime") == "" {
		useTimeRange = false
		fmt.Printf("Time range disabled - missing parameters\n")
	}

	var err error

	startTime := 0.0
	endTime := 0.0

	if useTimeRange {
		startTime, err = strconv.ParseFloat(r.FormValue("starttime"), 64)
		if err != nil {
			fmt.Printf("Error parsing starttime: %v\n", err)
			http.Error(w, "Invalid starttime parameter: "+err.Error(), http.StatusBadRequest)
			return
		}

		endTime, err = strconv.ParseFloat(r.FormValue("endtime"), 64)
		if err != nil {
			fmt.Printf("Error parsing endtime: %v\n", err)
			http.Error(w, "Invalid endtime parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		
		fmt.Printf("Parsed time range: %f to %f\n", startTime, endTime)
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
	}

	fmt.Printf("Query: ID='%s', ParentID='%s', Kind='%s', Where='%s', TimeRange=%v\n", 
		query.ID, query.ParentID, query.Kind, query.Where, query.EnableTimeRange)

	tasks := traceReader.ListTasks(r.Context(), query)
	
	fmt.Printf("Found %d tasks\n", len(tasks))
	if len(tasks) > 0 {
		fmt.Printf("First task: ID=%s, Kind=%s, Steps=%d\n", tasks[0].ID, tasks[0].Kind, len(tasks[0].Steps))
	}

	rsp, err := json.Marshal(tasks)
	if err != nil {
		fmt.Printf("Error marshaling tasks: %v\n", err)
		http.Error(w, "Failed to marshal tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	fmt.Printf("Response JSON length: %d bytes\n", len(rsp))

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(rsp)
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
		http.Error(w, "Failed to write response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	fmt.Printf("httpTrace completed successfully\n")
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

	// ListTasks queries tasks .
	ListTasks(ctx context.Context, query TaskQuery) []Task
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

	sort.Slice(components, func(i, j int) bool {
		return naturalLess(components[i], components[j])
	})

	// fmt.Printf("%v\n", components)

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

	var ptStartTime, ptEndTime sql.NullFloat64

	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
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

	if ptID.Valid {
		t.ParentTask.ID = ptID.String
		t.ParentTask.ParentID = ptParentID.String
		t.ParentTask.Kind = ptKind.String
		t.ParentTask.What = ptWhat.String
		t.ParentTask.Location = ptLocation.String
		t.ParentTask.StartTime = sim.VTimeInSec(ptStartTime.Float64)
		t.ParentTask.EndTime = sim.VTimeInSec(ptEndTime.Float64)
	}
}

func (r *SQLiteTraceReader) scanTaskWithoutParent(rows *sql.Rows, t *Task) {
	err := rows.Scan(
		&t.ID,
		&t.ParentID,
		&t.Kind,
		&t.What,
		&t.Location,
		&t.StartTime,
		&t.EndTime,
	)
	if err != nil {
		panic(err)
	}
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

func (*SQLiteTraceReader) addQueryConditionsToQueryStr(
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
		sqlStr += `
			AND t.Location = '` + query.Where + `'
		`
	}

	if query.EnableTimeRange {
		sqlStr += fmt.Sprintf(
			"AND t.EndTime > %.15f AND t.StartTime < %.15f",
			query.StartTime,
			query.EndTime)
	}

	return sqlStr
}

// MilestoneData represents milestone count data for a time window
type MilestoneData struct {
	Time           float64 `json:"time"`
	MilestoneCount int     `json:"milestone_count"`
}

// httpMilestones handles the API endpoint for querying milestone counts by time windows
func httpMilestones(w http.ResponseWriter, r *http.Request) {
	startTimeStr := r.FormValue("start_time")
	endTimeStr := r.FormValue("end_time")
	numWindowsStr := r.FormValue("num_windows")
	
	
	if startTimeStr == "" || endTimeStr == "" {
		http.Error(w, "start_time and end_time parameters required", http.StatusBadRequest)
		return
	}
	
	startTime, err := strconv.ParseFloat(startTimeStr, 64)
	if err != nil {
		http.Error(w, "Invalid start_time parameter", http.StatusBadRequest)
		return
	}
	
	endTime, err := strconv.ParseFloat(endTimeStr, 64)
	if err != nil {
		http.Error(w, "Invalid end_time parameter", http.StatusBadRequest)
		return
	}
	
	numWindows := 10 // default
	if numWindowsStr != "" {
		if parsed, err := strconv.Atoi(numWindowsStr); err == nil && parsed > 0 {
			numWindows = parsed
		}
	}
	
	milestoneData := traceReader.QueryMilestonesByTimeWindows(r.Context(), startTime, endTime, numWindows)
	
	rsp, err := json.Marshal(milestoneData)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(rsp)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// QueryMilestonesByTimeWindows queries milestone counts grouped by time windows
func (r *SQLiteTraceReader) QueryMilestonesByTimeWindows(ctx context.Context, startTime, endTime float64, numWindows int) []MilestoneData {
	duration := endTime - startTime
	windowDuration := duration / float64(numWindows)
	
	milestoneData := make([]MilestoneData, 0, numWindows)
	
	for i := 0; i < numWindows; i++ {
		windowStart := startTime + (float64(i) * windowDuration)
		windowEnd := startTime + (float64(i+1) * windowDuration)
		relativeTime := float64(i) * windowDuration
		
		// Query milestone count for this time window
		// For the last window, include the end boundary (<=) to avoid missing milestones at endTime
		var sqlStr string
		if i == numWindows-1 {
			// Last window: include the end boundary
			sqlStr = `
				SELECT COUNT(*) as milestone_count
				FROM trace_milestones 
				WHERE Time >= ? AND Time <= ?
			`
		} else {
			// Regular windows: exclude the end boundary
			sqlStr = `
				SELECT COUNT(*) as milestone_count
				FROM trace_milestones 
				WHERE Time >= ? AND Time < ?
			`
		}
		
		var count int
		err := r.QueryRowContext(ctx, sqlStr, windowStart, windowEnd).Scan(&count)
		if err != nil {
			count = 0
		}
		
		milestoneData = append(milestoneData, MilestoneData{
			Time:           relativeTime,
			MilestoneCount: count,
		})
	}
	
	return milestoneData
}

// ExecInfo represents execution information from the exec_info table
type ExecInfo struct {
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
}

// httpExecInfo handles the API endpoint for querying execution information
func httpExecInfo(w http.ResponseWriter, r *http.Request) {
	execInfo := traceReader.QueryExecInfo(r.Context())
	
	rsp, err := json.Marshal(execInfo)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(rsp)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// QueryExecInfo queries execution information from the exec_info table
func (r *SQLiteTraceReader) QueryExecInfo(ctx context.Context) *ExecInfo {
	execInfo := &ExecInfo{}
	
	rows, err := r.QueryContext(ctx, "SELECT Property, Value FROM exec_info")
	if err != nil {
		// If exec_info table doesn't exist, return empty exec info
		return execInfo
	}
	defer rows.Close()
	
	for rows.Next() {
		var property, value string
		err := rows.Scan(&property, &value)
		if err != nil {
			continue
		}
		
		switch property {
		case "Start Time":
			execInfo.StartTime = value
		case "End Time":
			execInfo.EndTime = value
		case "Command":
			execInfo.Command = value
		case "Working Directory":
			execInfo.WorkingDirectory = value
		}
	}
	
	return execInfo
}

// ComponentMilestoneData represents milestone count data for a component
type ComponentMilestoneData struct {
	Component      string `json:"component"`
	MilestoneCount int    `json:"milestone_count"`
}

// httpComponentMilestones handles the API endpoint for querying milestone counts by component
func httpComponentMilestones(w http.ResponseWriter, r *http.Request) {
	startTimeStr := r.FormValue("start_time")
	endTimeStr := r.FormValue("end_time")
	
	if startTimeStr == "" || endTimeStr == "" {
		http.Error(w, "start_time and end_time parameters required", http.StatusBadRequest)
		return
	}
	
	startTime, err := strconv.ParseFloat(startTimeStr, 64)
	if err != nil {
		http.Error(w, "Invalid start_time parameter", http.StatusBadRequest)
		return
	}
	
	endTime, err := strconv.ParseFloat(endTimeStr, 64)
	if err != nil {
		http.Error(w, "Invalid end_time parameter", http.StatusBadRequest)
		return
	}
	
	componentMilestoneData := traceReader.QueryMilestonesByComponent(r.Context(), startTime, endTime)
	
	rsp, err := json.Marshal(componentMilestoneData)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(rsp)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// QueryMilestonesByComponent queries milestone counts grouped by component location
func (r *SQLiteTraceReader) QueryMilestonesByComponent(ctx context.Context, startTime, endTime float64) []ComponentMilestoneData {
	sqlStr := `
		SELECT Location, COUNT(*) as milestone_count
		FROM trace_milestones 
		WHERE Time >= ? AND Time < ?
		GROUP BY Location
		ORDER BY milestone_count DESC
	`
	
	rows, err := r.QueryContext(ctx, sqlStr, startTime, endTime)
	if err != nil {
		// If trace_milestones table doesn't exist, return empty data
		return []ComponentMilestoneData{}
	}
	defer rows.Close()
	
	var componentData []ComponentMilestoneData
	
	for rows.Next() {
		var location string
		var count int
		
		err := rows.Scan(&location, &count)
		if err != nil {
			continue
		}
		
		componentData = append(componentData, ComponentMilestoneData{
			Component:      location,
			MilestoneCount: count,
		})
	}
	
	return componentData
}
