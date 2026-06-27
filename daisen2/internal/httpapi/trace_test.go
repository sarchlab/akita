package httpapi

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestInitReadOnlyReadsNonWALTrace is a regression test: a read-only connection
// must not try to set the journal mode. With the native driver "mode=ro" is a
// true read-only open, so the old "PRAGMA journal_mode=WAL" in InitReadOnly
// failed on any non-WAL trace file and panicked.
func TestInitReadOnlyReadsNonWALTrace(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")

	// Create the trace in the default rollback-journal mode (not WAL), so any
	// attempt by the read-only connection to change the journal mode would fail.
	writeDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if _, err := writeDB.Exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location TEXT, StartTime REAL, EndTime REAL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writeDB.Exec(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		VALUES (1, 0, 'req_in', 'ReadReq', 'DRAM', 1000, 9000)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := writeDB.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader := NewSQLiteTraceReader(dbPath)
	reader.InitReadOnly() // must not panic
	defer reader.Close()

	timeRange, ok := reader.TimeRange(context.Background())
	if !ok {
		t.Fatal("expected a trace time range from the read-only reader")
	}
	if timeRange.StartTime != 1000 || timeRange.EndTime != 9000 {
		t.Fatalf("unexpected time range: %+v", timeRange)
	}
}

func TestSQLiteTraceReaderTimeRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	defer reader.Close()

	_, err := reader.Exec(`CREATE TABLE trace (
		ID INTEGER,
		ParentID INTEGER,
		Kind TEXT,
		What TEXT,
		Location TEXT,
		StartTime REAL,
		EndTime REAL
	)`)
	if err != nil {
		t.Fatalf("create trace table: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		VALUES
		(1, 0, 'req_in', 'ReadReq', 'DRAM', 2000, 6000),
		(2, 0, 'req_in', 'WriteReq', 'DRAM', 1000, 9000)`)
	if err != nil {
		t.Fatalf("insert trace rows: %v", err)
	}

	timeRange, ok := reader.TimeRange(context.Background())
	if !ok {
		t.Fatal("expected a trace time range")
	}
	if timeRange.StartTime != 1000 || timeRange.EndTime != 9000 {
		t.Fatalf("unexpected time range: %+v", timeRange)
	}
}

func TestSQLiteTraceReaderMergesTagsAndMilestonesIntoSteps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	defer reader.Close()

	exec := func(query string) {
		if _, err := reader.Exec(query); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	// Location is interned: trace.Location holds an id into the location table.
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`CREATE TABLE location (ID INTEGER, Locale TEXT)`)
	exec(`CREATE TABLE milestone (
		ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	exec(`CREATE TABLE tag (ID INTEGER, TaskID INTEGER, Time REAL, What TEXT)`)

	exec(`INSERT INTO trace VALUES (1, 0, 'req_in', 'ReadReq', 1, 2000, 6000)`)
	exec(`INSERT INTO location VALUES (1, 'DRAM')`)
	exec(`INSERT INTO milestone VALUES (10, 1, 5000, 'data', 'arrived')`)
	exec(`INSERT INTO tag VALUES (20, 1, 3000, 'read-hit')`)

	tasks := reader.ListTasks(context.Background(),
		TaskQuery{ID: 1, EnableMilestones: true})

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.Location != "DRAM" {
		t.Fatalf("expected location DRAM, got %q", task.Location)
	}

	// Steps merge the tag (t=3000) and the milestone (t=5000), time-ordered, so
	// the persisted tag is no longer dropped from task details.
	if len(task.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d: %+v", len(task.Steps), task.Steps)
	}

	if task.Steps[0].Kind != "tag" || task.Steps[0].What != "read-hit" {
		t.Fatalf("expected first step to be the read-hit tag, got %+v",
			task.Steps[0])
	}

	if task.Steps[1].Kind != "data" || task.Steps[1].What != "arrived" {
		t.Fatalf("expected second step to be the data milestone, got %+v",
			task.Steps[1])
	}
}

func TestSQLiteTraceReaderTimeRangePrefersExecInfoVirtualTime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	defer reader.Close()

	_, err := reader.Exec(`CREATE TABLE trace (
		ID INTEGER,
		ParentID INTEGER,
		Kind TEXT,
		What TEXT,
		Location TEXT,
		StartTime REAL,
		EndTime REAL
	)`)
	if err != nil {
		t.Fatalf("create trace table: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		VALUES
		(1, 0, 'req_in', 'ReadReq', 'DRAM', 2000, 6000)`)
	if err != nil {
		t.Fatalf("insert trace rows: %v", err)
	}

	_, err = reader.Exec(`CREATE TABLE exec_info (Property TEXT, Value TEXT)`)
	if err != nil {
		t.Fatalf("create exec_info table: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO exec_info (Property, Value)
		VALUES
		('Start Virtual Time', '0'),
		('End Virtual Time', '9000')`)
	if err != nil {
		t.Fatalf("insert exec_info rows: %v", err)
	}

	timeRange, ok := reader.TimeRange(context.Background())
	if !ok {
		t.Fatal("expected a trace time range")
	}
	if timeRange.StartTime != 0 || timeRange.EndTime != 9000 {
		t.Fatalf("unexpected time range: %+v", timeRange)
	}
}

// TestListTasksScopeAggregatesSubtree verifies a Scope task query returns the
// scope's own tasks plus everything nested under it (the dotted subtree), excludes
// a same-prefix sibling like "ROBOT", and honors the time-range overlap filter.
// This locks in the location-id sub-select form of the Scope/time predicate, which
// keeps the query probing trace by its Location index instead of driving from a
// time-range index (the latter scanned every task after StartTime — ~25s on a real
// trace for what is a handful of in-scope rows).
func TestListTasksScopeAggregatesSubtree(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	defer reader.Close()

	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE location (ID INTEGER, Locale TEXT)`)
	exec(`INSERT INTO location (ID, Locale) VALUES
		(1, 'ROB'), (2, 'ROB.req_in'), (3, 'ROBOT.x')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`CREATE TABLE milestone (
		ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	exec(`CREATE TABLE tag (ID INTEGER, TaskID INTEGER, Time REAL, What TEXT)`)
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'k', 'a', 1, 0, 10),
		(2, 0, 'k', 'b', 2, 0, 10),
		(3, 0, 'k', 'c', 3, 0, 10),
		(4, 0, 'k', 'd', 2, 100, 110)`)

	tasks := reader.ListTasks(context.Background(), TaskQuery{
		Scope:            "ROB",
		StartTime:        0,
		EndTime:          50,
		EnableTimeRange:  true,
		EnableMilestones: true,
	})

	// ROB + ROB.req_in overlapping [0,50): tasks 1 and 2. The sibling "ROBOT.x"
	// (task 3) is excluded by the dot boundary; task 4 (ROB.req_in but at t=100)
	// is excluded by the time window.
	if len(tasks) != 2 {
		t.Fatalf("scope ROB returned %d tasks, want 2 (ids 1,2)", len(tasks))
	}
	got := map[uint64]bool{}
	for _, tk := range tasks {
		got[tk.ID] = true
	}
	if !got[1] || !got[2] {
		t.Fatalf("scope ROB task ids = %v, want {1,2}", got)
	}

	// A leaf scope matches only itself, not the subtree.
	leaf := reader.ListTasks(context.Background(), TaskQuery{
		Scope:           "ROB.req_in",
		StartTime:       0,
		EndTime:         50,
		EnableTimeRange: true,
	})
	if len(leaf) != 1 || leaf[0].ID != 2 {
		t.Fatalf("scope ROB.req_in = %+v, want exactly task id 2", leaf)
	}
}
