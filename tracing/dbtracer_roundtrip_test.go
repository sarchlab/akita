package tracing

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/timing"
)

// roundTripDomain is a minimal NamedHookable used to drive the DBTracer through
// a real emit -> persist cycle.
type roundTripDomain struct {
	*hooking.HookableBase
	name string
	now  timing.VTimeInPicoSec
}

func (d *roundTripDomain) Name() string                       { return d.name }
func (d *roundTripDomain) CurrentTime() timing.VTimeInPicoSec { return d.now }

// TestDBTracerLocationRoundTrip writes a trace through the real DBTracer and
// reads it back with the same SQL shape daisen uses, confirming that the
// interned location id resolves to the task's single-kind location (a req_in
// task lands at "<component>.req_in") and that milestones and tags inherit the
// task's location.
func TestDBTracerLocationRoundTrip(t *testing.T) {
	dbFile := writeRoundTripTrace(t)

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	assertTaskLocation(t, db)
	assertDictionaryAndChildren(t, db)
}

// writeRoundTripTrace emits one task (with a tag and a milestone) through the
// real DBTracer and returns the path of the SQLite file it produced.
func writeRoundTripTrace(t *testing.T) string {
	t.Helper()

	dbName := "roundtrip_loc_test"
	dbFile := dbName + ".sqlite3"
	os.Remove(dbFile)
	t.Cleanup(func() { os.Remove(dbFile) })

	recorder := datarecording.NewDataRecorder(dbName)
	domain := &roundTripDomain{
		HookableBase: hooking.NewHookableBase(),
		name:         "GPU[0].L1Cache",
	}

	tracer := NewDBTracer(domain, recorder)
	CollectTrace(domain, tracer)
	tracer.StartTracing()

	domain.now = 10
	StartTask(domain, TaskStart{ID: 1, Kind: "req_in", What: "ReadReq"})
	domain.now = 12
	AddTaskTag(domain, TaskTag{TaskID: 1, What: "read-hit"})
	AddMilestone(domain, Milestone{TaskID: 1, Kind: MilestoneKindQueue, What: "queued"})
	domain.now = 20
	EndTask(domain, TaskEnd{ID: 1})

	tracer.StopTracing()
	tracer.Terminate()

	return dbFile
}

// assertTaskLocation checks that trace.Location (an interned id) joins back to
// the task's single-kind location and that the task times round-trip.
func assertTaskLocation(t *testing.T, db *sql.DB) {
	t.Helper()

	var id uint64
	var location string
	var start, end float64
	err := db.QueryRow(`
		SELECT t.ID, loc.Locale, t.StartTime, t.EndTime
		FROM trace t JOIN location loc ON t.Location = loc.ID
		WHERE t.ID = 1`).Scan(&id, &location, &start, &end)
	if err != nil {
		t.Fatalf("query trace: %v", err)
	}
	if location != "GPU[0].L1Cache.req_in" {
		t.Fatalf("location = %q, want GPU[0].L1Cache.req_in", location)
	}
	if start != 10 || end != 20 {
		t.Fatalf("times = (%v,%v), want (10,20)", start, end)
	}
}

// assertDictionaryAndChildren checks the location dictionary plus the milestone
// and tag rows, neither of which stores its own location.
func assertDictionaryAndChildren(t *testing.T, db *sql.DB) {
	t.Helper()

	var locCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM location`).Scan(&locCount); err != nil {
		t.Fatalf("count location: %v", err)
	}
	if locCount != 1 {
		t.Fatalf("location rows = %d, want 1", locCount)
	}

	var mWhat string
	if err := db.QueryRow(`SELECT What FROM milestone WHERE TaskID = 1`).Scan(&mWhat); err != nil {
		t.Fatalf("query milestone: %v", err)
	}
	if mWhat != "queued" {
		t.Fatalf("milestone What = %q, want queued", mWhat)
	}

	var tagWhat string
	if err := db.QueryRow(`SELECT What FROM tag WHERE TaskID = 1`).Scan(&tagWhat); err != nil {
		t.Fatalf("query tag: %v", err)
	}
	if tagWhat != "read-hit" {
		t.Fatalf("tag What = %q, want read-hit", tagWhat)
	}
}
