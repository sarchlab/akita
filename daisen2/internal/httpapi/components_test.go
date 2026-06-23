package httpapi

import (
	"context"
	"testing"
)

func TestComponentsByResidencyRanksByTaskTime(t *testing.T) {
	reader := newTestReader(t)
	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE location (ID INTEGER, Locale TEXT)`)
	exec(`INSERT INTO location VALUES (1, 'A'), (2, 'B')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	// A: one 30-long task. B: two tasks, 40 + 90 = 130 -> B ranks first by total
	// in-flight task time.
	exec(`INSERT INTO trace VALUES (1, 0, 'req_in', 'R', 1, 0, 30)`)
	exec(`INSERT INTO trace VALUES (2, 0, 'req_in', 'R', 2, 0, 40)`)
	exec(`INSERT INTO trace VALUES (3, 0, 'req_in', 'R', 2, 10, 100)`)

	ranked := reader.ComponentsByResidency(context.Background())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 components, got %d (%+v)", len(ranked), ranked)
	}
	if ranked[0].Component != "B" || ranked[0].TaskTime != 130 {
		t.Fatalf("expected B=130 first, got %+v", ranked[0])
	}
	if ranked[1].Component != "A" || ranked[1].TaskTime != 30 {
		t.Fatalf("expected A=30 second, got %+v", ranked[1])
	}
}

func TestComponentsByResidencyMissingTables(t *testing.T) {
	reader := newTestReader(t)
	ranked := reader.ComponentsByResidency(context.Background())
	if ranked == nil || len(ranked) != 0 {
		t.Fatalf("expected empty slice for missing tables, got %#v", ranked)
	}
}
