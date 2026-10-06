package httpapi

import (
	"context"
	"path/filepath"
	"testing"
)

// seedBlockingTrace builds a small trace whose hardware_resource milestones drive
// the count ranking: bankA fires 3 times across 2 tasks, portX twice in 1 task,
// bankB once; a non-hardware_resource milestone must be ignored.
func seedBlockingTrace(t *testing.T) *SQLiteTraceReader {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	t.Cleanup(func() { reader.Close() })

	exec := func(query string) {
		if _, err := reader.Exec(query); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`CREATE TABLE location (ID INTEGER, Locale TEXT)`)
	exec(`CREATE TABLE milestone (ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	exec(`INSERT INTO location VALUES (1, 'DRAM'), (2, 'Cache')`)
	exec(`INSERT INTO trace VALUES
		(1, 0, 'req_in', 'R', 1, 0, 100),
		(2, 0, 'req_in', 'R', 1, 0, 100),
		(3, 0, 'req_in', 'R', 2, 0, 100),
		(4, 0, 'req_in', 'R', 1, 0, 100)`)
	exec(`INSERT INTO milestone VALUES
		(10, 1, 20, 'hardware_resource', 'bankA'),
		(11, 1, 60, 'hardware_resource', 'bankA'),
		(12, 1, 50, 'data', 'arrived'),
		(13, 2, 40, 'hardware_resource', 'bankA'),
		(14, 3, 70, 'hardware_resource', 'portX'),
		(15, 3, 80, 'hardware_resource', 'portX'),
		(16, 4, 10, 'hardware_resource', 'bankB')`)

	return reader
}

func TestTopBlockingResourcesGlobalRanking(t *testing.T) {
	reader := seedBlockingTrace(t)

	resp := reader.TopBlockingResources(context.Background(), "", 10)

	// bankA (3 events / 2 tasks) > portX (2 / 1) > bankB (1 / 1); 'data' excluded.
	want := []BlockingResource{
		{What: "bankA", Count: 3, TaskCount: 2},
		{What: "portX", Count: 2, TaskCount: 1},
		{What: "bankB", Count: 1, TaskCount: 1},
	}
	if len(resp.Resources) != len(want) {
		t.Fatalf("expected %d resources, got %d: %+v", len(want), len(resp.Resources), resp.Resources)
	}
	for i, w := range want {
		if resp.Resources[i] != w {
			t.Errorf("rank %d = %+v, want %+v", i, resp.Resources[i], w)
		}
	}
}

func TestTopBlockingResourcesScopeFilter(t *testing.T) {
	reader := seedBlockingTrace(t)

	// Scoped to DRAM: only tasks 1, 2, 4 — portX (Cache) must be excluded.
	resp := reader.TopBlockingResources(context.Background(), "DRAM", 10)

	want := []BlockingResource{
		{What: "bankA", Count: 3, TaskCount: 2},
		{What: "bankB", Count: 1, TaskCount: 1},
	}
	if len(resp.Resources) != len(want) {
		t.Fatalf("expected %d DRAM resources, got %d: %+v", len(want), len(resp.Resources), resp.Resources)
	}
	for i, w := range want {
		if resp.Resources[i] != w {
			t.Errorf("DRAM rank %d = %+v, want %+v", i, resp.Resources[i], w)
		}
	}
}

func TestTopBlockingResourcesLimit(t *testing.T) {
	reader := seedBlockingTrace(t)

	resp := reader.TopBlockingResources(context.Background(), "", 1)

	if len(resp.Resources) != 1 || resp.Resources[0].What != "bankA" {
		t.Fatalf("limit=1 should return only bankA, got %+v", resp.Resources)
	}
}
