package httpapi

import (
	"context"
	"path/filepath"
	"testing"
)

// TestComponentTimelineScopeAggregatesSubtree verifies that a scope aggregates a
// whole location subtree (the exact location plus everything dotted beneath it),
// while a leaf scope matches only itself — and that the dot boundary keeps the
// prefix from leaking into a sibling like "ROBOT".
func TestComponentTimelineScopeAggregatesSubtree(t *testing.T) {
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
		(1, 'ROB.req_in'), (2, 'ROB.req_out'), (3, 'ROBOT.x')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in',  'ReadReq', 1, 0, 10),
		(2, 0, 'req_out', 'ReadReq', 2, 0, 10),
		(3, 0, 'misc',    'Other',   3, 0, 10)`)

	// scope "ROB" aggregates its two children but NOT the sibling "ROBOT.x".
	sub := reader.ComponentTimeline(context.Background(), "ROB", 0, 10, 1, false, 1)
	if sub.Total != 2 {
		t.Fatalf("scope ROB Total = %d, want 2 (req_in + req_out, not ROBOT.x)", sub.Total)
	}
	if len(sub.Keys) != 2 {
		t.Fatalf("scope ROB keys = %v, want 2 distinct", sub.Keys)
	}

	// A leaf scope matches only itself.
	leaf := reader.ComponentTimeline(context.Background(), "ROB.req_in", 0, 10, 1, false, 1)
	if leaf.Total != 1 {
		t.Fatalf("scope ROB.req_in Total = %d, want 1", leaf.Total)
	}
}

// TestCountTasksInScope verifies the cheap exact count that guards the exact
// occupancy scan: it counts tasks overlapping the range in a location subtree
// (matching ComponentTimeline's Total), excludes a sibling prefix, and respects
// the time bounds.
func TestCountTasksInScope(t *testing.T) {
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
		(1, 'ROB.req_in'), (2, 'ROB.req_out'), (3, 'ROBOT.x')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in',  'ReadReq', 1, 0, 10),
		(2, 0, 'req_out', 'ReadReq', 2, 5, 20),
		(3, 0, 'misc',    'Other',   3, 0, 10)`)

	ctx := context.Background()

	// Subtree scope counts both ROB children, excludes the sibling ROBOT.x, and
	// matches ComponentTimeline's Total over the same range.
	n, ok := reader.countTasksInScope(ctx, "ROB", 0, 30)
	if !ok || n != 2 {
		t.Fatalf("countTasksInScope(ROB) = %d, ok=%v, want 2", n, ok)
	}
	if got := reader.ComponentTimeline(ctx, "ROB", 0, 30, 4, false, 1).Total; got != n {
		t.Fatalf("count %d disagrees with ComponentTimeline Total %d", n, got)
	}

	// A leaf scope matches only itself.
	if n, ok := reader.countTasksInScope(ctx, "ROB.req_in", 0, 30); !ok || n != 1 {
		t.Fatalf("countTasksInScope(ROB.req_in) = %d, ok=%v, want 1", n, ok)
	}

	// The time window [0, 4) excludes the task that starts at 5.
	if n, ok := reader.countTasksInScope(ctx, "ROB", 0, 4); !ok || n != 1 {
		t.Fatalf("countTasksInScope(ROB, 0..4) = %d, ok=%v, want 1", n, ok)
	}
}

// TestComponentTimelineGroupByKind verifies that groupByKind collapses tasks that
// share a kind but differ in what into one band, while the default kind-what
// grouping keeps them distinct.
func TestComponentTimelineGroupByKind(t *testing.T) {
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
	exec(`INSERT INTO location (ID, Locale) VALUES (1, 'L1.bank')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	// Two req_in tasks differing only in What, plus one req_out.
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in',  'ReadReq',  1, 0, 10),
		(2, 0, 'req_in',  'WriteReq', 1, 0, 10),
		(3, 0, 'req_out', 'ReadReq',  1, 0, 10)`)

	kindWhat := reader.ComponentTimeline(context.Background(), "L1.bank", 0, 10, 1, false, 1)
	if len(kindWhat.Keys) != 3 {
		t.Fatalf("kind-what keys = %v, want 3 distinct", kindWhat.Keys)
	}

	byKind := reader.ComponentTimeline(context.Background(), "L1.bank", 0, 10, 1, true, 1)
	if len(byKind.Keys) != 2 {
		t.Fatalf("by-kind keys = %v, want 2 (req_in, req_out)", byKind.Keys)
	}
	if byKind.Keys[0] != "req_in" || byKind.Keys[1] != "req_out" {
		t.Fatalf("by-kind keys = %v, want [req_in req_out]", byKind.Keys)
	}
	// Occupancy is preserved: both groupings cover the same three tasks.
	if byKind.Total != 3 || kindWhat.Total != 3 {
		t.Fatalf("Total mismatch: byKind=%d kindWhat=%d, want 3 each", byKind.Total, kindWhat.Total)
	}
}

// TestComponentTimelineRespectsHalfOpenBins verifies that a task whose EndTime
// lands exactly on a bin boundary is counted only in the bins it actually
// overlaps, not the next one. A task's [start, end) interval is half-open, so a
// task ending on a boundary must not leave phantom occupancy in the following
// bin (regression for the floor+1 vs ceil end-bin bug).
func TestComponentTimelineRespectsHalfOpenBins(t *testing.T) {
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
	exec(`INSERT INTO location (ID, Locale) VALUES (1, 'ROB')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	// Over [0,20) with 2 bins (each 10 wide):
	//   A [0,10) ends exactly on the bin-0/bin-1 boundary -> active in bin 0 only.
	//   B [0,20) spans the whole range                    -> active in bins 0 and 1.
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in', 'ReadReq', 1, 0, 10),
		(2, 0, 'req_in', 'ReadReq', 1, 0, 20)`)

	resp := reader.ComponentTimeline(context.Background(), "ROB", 0, 20, 2, false, 1)

	if resp.Total != 2 {
		t.Fatalf("Total = %d, want 2", resp.Total)
	}
	if len(resp.Bins) != 2 {
		t.Fatalf("len(Bins) = %d, want 2", len(resp.Bins))
	}

	sum := func(row []int) int {
		total := 0
		for _, v := range row {
			total += v
		}
		return total
	}
	if got := sum(resp.Bins[0]); got != 2 {
		t.Fatalf("bin 0 occupancy = %d, want 2 (A and B both active)", got)
	}
	if got := sum(resp.Bins[1]); got != 1 {
		t.Fatalf("bin 1 occupancy = %d, want 1 (only B; A must not bleed past its boundary)", got)
	}
}

// TestBlockingReasonOccupancyBinsMilestoneIntervals verifies the blocking-reason
// chart uses the same occupancy binning as the task-count chart, but over each
// task's per-milestone intervals: a milestone marks the release of a blocking
// reason, so the interval ending at it (from the previous milestone, or the
// task's start) is time spent blocked on that milestone's kind. It also confirms
// the scope aggregates the location subtree and excludes a sibling like "ROBOT".
func TestBlockingReasonOccupancyBinsMilestoneIntervals(t *testing.T) {
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
	exec(`INSERT INTO location (ID, Locale) VALUES (1, 'ROB.req_in'), (2, 'ROBOT.x')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in', 'ReadReq', 1, 0, 20),
		(2, 0, 'misc',   'Other',   2, 0, 20)`)
	exec(`CREATE TABLE milestone (ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	// Task 1 (in scope): blocked on "queue" until t=10, then on "data" until t=20.
	// Task 2 (sibling ROBOT.x): must be excluded from scope "ROB".
	exec(`INSERT INTO milestone (ID, TaskID, Time, Kind, What) VALUES
		(1, 1, 10, 'queue', ''),
		(2, 1, 20, 'data',  ''),
		(3, 2, 10, 'queue', '')`)

	keys, bins := reader.BlockingReasonOccupancy(context.Background(), "ROB", 0, 20, 2, 1)

	if len(keys) != 2 || keys[0] != "data" || keys[1] != "queue" {
		t.Fatalf("keys = %v, want [data queue]", keys)
	}
	di, qi := 0, 1 // keys are sorted: data, queue

	// Over [0,20) with 2 bins (each 10 wide):
	//   queue interval (0,10]  -> active in bin 0 only.
	//   data  interval (10,20] -> active in bin 1 only.
	// The sibling ROBOT.x "queue" milestone must not lift bin 0's queue above 1.
	if bins[0][qi] != 1 || bins[0][di] != 0 {
		t.Fatalf("bin 0 = {data:%d queue:%d}, want {data:0 queue:1}", bins[0][di], bins[0][qi])
	}
	if bins[1][di] != 1 || bins[1][qi] != 0 {
		t.Fatalf("bin 1 = {data:%d queue:%d}, want {data:1 queue:0}", bins[1][di], bins[1][qi])
	}
}

// TestComponentTimelineScopeIsCaseSensitive verifies the scope subtree match is
// case-sensitive, matching the case-sensitive location tree and exact `=` lookup.
// A case-insensitive LIKE would pull a differently-cased sibling (here lowercase
// "tlb.req_in") into scope "TLB" and show counts for the wrong component.
func TestComponentTimelineScopeIsCaseSensitive(t *testing.T) {
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
	exec(`INSERT INTO location (ID, Locale) VALUES (1, 'TLB.req_in'), (2, 'tlb.req_in')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in', 'ReadReq', 1, 0, 10),
		(2, 0, 'req_in', 'ReadReq', 2, 0, 10)`)

	resp := reader.ComponentTimeline(context.Background(), "TLB", 0, 10, 1, false, 1)
	if resp.Total != 1 {
		t.Fatalf("scope TLB Total = %d, want 1 (case-sensitive: excludes tlb.req_in)", resp.Total)
	}
}
