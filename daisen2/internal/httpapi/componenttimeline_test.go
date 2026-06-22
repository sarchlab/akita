package httpapi

import (
	"context"
	"path/filepath"
	"testing"
)

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

	resp := reader.ComponentTimeline(context.Background(), "ROB", 0, 20, 2)

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
