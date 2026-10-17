package tracing

import (
	"os"
	"testing"

	"database/sql"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// roundTripDomain is a minimal NamedHookable used to drive the DBTracer through
// a real emit -> persist cycle.
type roundTripDomain struct {
	ids     timing.IDGenerator
	tracker *tasks.Tracker
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

	recorder := datarecording.NewDataRecorder(dbName + ".sqlite3")
	domain := &roundTripDomain{tracker: tasks.NewTracker(),
		HookableBase: &hooking.HookableBase{},
		name:         "GPU[0].L1Cache",
	}

	tracer := NewDBTracer(domain, recorder)
	domain.TaskTracker().Subscribe(tracer, domain)
	tracer.StartTracing()

	domain.now = 10
	tasks.StartTask(domain, tasks.StartInfo{Kind: "req_in", What: "ReadReq"})
	domain.now = 12
	tasks.AddTag(domain, tasks.Tag{TaskID: 1, What: "read-hit"})
	tasks.AddMilestone(domain, tasks.Milestone{TaskID: 1, Kind: tasks.MilestoneKindQueue, What: "queued"})
	domain.now = 20
	tasks.EndTask(domain, tasks.EndInfo{ID: 1})

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

func (d *roundTripDomain) NewID() uint64 { return d.ids.NewID() }

func (d *roundTripDomain) TaskTracker() *tasks.Tracker {
	return d.tracker
}

func TestDBRecordsCompleteEndWithoutStartOrHistory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "late")
	backend := datarecording.NewDataRecorder(base + ".sqlite3")
	clock := &testTimeTeller{}
	tracer := NewDBTracer(clock, backend)
	task := tasks.Task{ID: 10, MsgID: 5, ParentID: 2, Kind: tasks.KindReqIn, What: "late",
		Location: "Owner.req_in", StartTime: 10, EndTime: 40}
	tracer.TaskStarted(task)
	tracer.MilestoneAdded(tasks.Milestone{ID: 20, TaskID: 10, Time: 15, What: "before"})
	if len(tracer.tracingTasks) != 0 {
		t.Fatal("idle DB observer retained task history")
	}
	clock.SetCurrentTime(30)
	tracer.StartTracing()
	tracer.MilestoneAdded(tasks.Milestone{ID: 21, TaskID: 10, Time: 35, What: "during"})
	tracer.TaskEnded(task)
	clock.SetCurrentTime(40)
	tracer.StopTracing()
	tracer.Terminate()
	backend.Close()
	db, err := sql.Open("sqlite", base+".sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var parent uint64
	var start, end float64
	row := db.QueryRow("SELECT ParentID, StartTime, EndTime FROM trace WHERE ID=10")
	if err := row.Scan(&parent, &start, &end); err != nil {
		t.Fatal(err)
	}
	if parent != 2 || start != 10 || end != 40 {
		t.Fatalf("incomplete end record: %d %v %v", parent, start, end)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM milestone").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected only the milestone observed during recording, got %d", count)
	}
	t.Log("SQLite recorded parent=2 and original interval [10,40] from an end notification, with no historical backfill")
}
