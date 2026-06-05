package httpapi

import (
	"context"
	"path/filepath"
	"testing"
)

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
