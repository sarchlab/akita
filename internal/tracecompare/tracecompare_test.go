package tracecompare

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/sqlitepath"
)

type taskRow struct {
	id, parent uint64
	what       string
	start, end float64
}

// writeTrace writes a minimal trace database. Each task gets one milestone
// whose ID is the task ID plus milestoneIDOffset.
func writeTrace(
	t *testing.T,
	name string,
	rows []taskRow,
	milestoneIDOffset uint64,
) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	db, err := sql.Open("sqlite", sqlitepath.URI(path, "rwc"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE "akita$location" (ID INTEGER, Locale TEXT)`,
		`CREATE TABLE "akita$trace" (ID INTEGER, ParentID INTEGER, Kind TEXT,
			What TEXT, Location INTEGER, StartTime REAL, EndTime REAL)`,
		`CREATE TABLE "akita$milestone" (ID INTEGER, TaskID INTEGER,
			Time REAL, Kind TEXT, What TEXT)`,
		`CREATE TABLE "akita$tag" (ID INTEGER, TaskID INTEGER, Time REAL,
			What TEXT)`,
		`INSERT INTO "akita$location" VALUES (1, 'Cache'), (2, 'DRAM')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}

	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO "akita$trace" VALUES (?,?,?,?,?,?,?)`,
			r.id, r.parent, "req_in", r.what, 1+r.id%2, r.start, r.end); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec(`INSERT INTO "akita$milestone" VALUES (?,?,?,?,?)`,
			r.id+milestoneIDOffset, r.id, r.start, "queue", "buf"); err != nil {
			t.Fatal(err)
		}
	}

	return path
}

func mustCompare(t *testing.T, a, b string, mode Mode) Result {
	t.Helper()

	r, err := Compare(a, b, mode)
	if err != nil {
		t.Fatal(err)
	}

	return r
}

func TestIdenticalTracesMatchExactly(t *testing.T) {
	rows := []taskRow{{1, 0, "Read", 0, 10}, {2, 1, "Write", 1, 5}}
	a := writeTrace(t, "a.sqlite3", rows, 100)
	b := writeTrace(t, "b.sqlite3", rows, 100)

	if r := mustCompare(t, a, b, Exact); !r.Equal() {
		t.Fatalf("expected equal traces, got %d diffs: %v", r.NumDiffs, r.Diffs)
	}
}

func TestRenumberedTracesMatchOnlyWithoutIDs(t *testing.T) {
	// The same two tasks with different IDs. Location derives from the ID, so
	// the renumbering keeps each task's location.
	a := writeTrace(t, "a.sqlite3",
		[]taskRow{{1, 0, "Read", 0, 10}, {2, 1, "Write", 1, 5}}, 100)
	b := writeTrace(t, "b.sqlite3",
		[]taskRow{{5, 0, "Read", 0, 10}, {8, 5, "Write", 1, 5}}, 200)

	if r := mustCompare(t, a, b, IgnoreIDs); !r.Equal() {
		t.Fatalf("expected ID-free match, got %d diffs: %v", r.NumDiffs, r.Diffs)
	}

	if r := mustCompare(t, a, b, Exact); r.Equal() {
		t.Fatal("expected exact comparison to report the renumbering")
	}
}

func TestTimingChangeIsReportedInBothModes(t *testing.T) {
	a := writeTrace(t, "a.sqlite3",
		[]taskRow{{1, 0, "Read", 0, 10}, {2, 1, "Write", 1, 5}}, 100)
	b := writeTrace(t, "b.sqlite3",
		[]taskRow{{1, 0, "Read", 0, 10}, {2, 1, "Write", 1, 6}}, 100)

	for _, mode := range []Mode{Exact, IgnoreIDs} {
		if r := mustCompare(t, a, b, mode); r.Equal() {
			t.Fatalf("mode %d: expected the end-time change to be reported", mode)
		}
	}
}

func TestReparentingIsReportedWithoutIDs(t *testing.T) {
	// Task "Write" moves from under "Read" to under "Flush".
	a := writeTrace(t, "a.sqlite3", []taskRow{
		{1, 0, "Read", 0, 10}, {2, 0, "Flush", 0, 10}, {3, 1, "Write", 1, 5},
	}, 100)
	b := writeTrace(t, "b.sqlite3", []taskRow{
		{1, 0, "Read", 0, 10}, {2, 0, "Flush", 0, 10}, {3, 2, "Write", 1, 5},
	}, 100)

	if r := mustCompare(t, a, b, IgnoreIDs); r.Equal() {
		t.Fatal("expected the parent change to be reported")
	}
}

func TestChildReassignmentBetweenIdenticalParentsIsReported(t *testing.T) {
	// P1 and P2 (IDs 1 and 3) have identical content: same what, times,
	// location (odd IDs), and milestone. Only their children differ.
	a := writeTrace(t, "a.sqlite3", []taskRow{
		{1, 0, "P", 0, 10}, {3, 0, "P", 0, 10},
		{5, 1, "Write", 1, 5}, {7, 3, "Flush", 1, 5},
	}, 100)
	b := writeTrace(t, "b.sqlite3", []taskRow{
		{1, 0, "P", 0, 10}, {3, 0, "P", 0, 10},
		{5, 1, "Write", 1, 5}, {7, 1, "Flush", 1, 5},
	}, 100)

	if r := mustCompare(t, a, b, IgnoreIDs); r.Equal() {
		t.Fatal("expected the child reassignment to be reported")
	}
}

func TestRenumberedForestWithIdenticalParentsMatches(t *testing.T) {
	// The same forest as above (each P has one child), with IDs swapped.
	a := writeTrace(t, "a.sqlite3", []taskRow{
		{1, 0, "P", 0, 10}, {3, 0, "P", 0, 10},
		{5, 1, "Write", 1, 5}, {7, 3, "Flush", 1, 5},
	}, 100)
	b := writeTrace(t, "b.sqlite3", []taskRow{
		{3, 0, "P", 0, 10}, {1, 0, "P", 0, 10},
		{5, 3, "Write", 1, 5}, {7, 1, "Flush", 1, 5},
	}, 200)

	if r := mustCompare(t, a, b, IgnoreIDs); !r.Equal() {
		t.Fatalf("expected a match, got %d diffs: %v", r.NumDiffs, r.Diffs)
	}
}
