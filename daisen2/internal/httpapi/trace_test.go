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
