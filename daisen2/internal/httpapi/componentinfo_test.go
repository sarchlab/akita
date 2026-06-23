package httpapi

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func newTestTraceReader(t *testing.T) *SQLiteTraceReader {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	t.Cleanup(func() {
		_ = reader.Close()
	})

	// Location is stored as an interned integer id that references the shared
	// location table, matching what DBTracer writes.
	_, err := reader.Exec(`CREATE TABLE trace (
		ID INTEGER,
		ParentID INTEGER,
		Kind TEXT,
		What TEXT,
		Location INTEGER,
		StartTime REAL,
		EndTime REAL
	)`)
	if err != nil {
		t.Fatalf("create trace table: %v", err)
	}

	_, err = reader.Exec(`CREATE TABLE location (
		ID INTEGER,
		Locale TEXT
	)`)
	if err != nil {
		t.Fatalf("create location table: %v", err)
	}

	// The milestone table no longer stores Location; it is inherited from the
	// owning task.
	_, err = reader.Exec(`CREATE TABLE milestone (
		ID INTEGER,
		TaskID INTEGER,
		Time REAL,
		Kind TEXT,
		What TEXT
	)`)
	if err != nil {
		t.Fatalf("create milestone table: %v", err)
	}

	return reader
}

func insertTraceRows(t *testing.T, reader *SQLiteTraceReader) {
	t.Helper()

	// Intern the component names: A=1, B=2, P=3, C=4.
	_, err := reader.Exec(`INSERT INTO location (ID, Locale)
		VALUES (1, 'A'), (2, 'B'), (3, 'P'), (4, 'C')`)
	if err != nil {
		t.Fatalf("insert location rows: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		VALUES
		(1, 0, 'req_in', 'Req1', 1, 10, 20),
		(2, 0, 'req_in', 'Req2', 1, 20, 40),
		(3, 0, 'req_in', 'Req3', 1, 35, 45),
		(4, 0, 'req_out', 'Out1', 1, 0, 10),
		(5, 0, 'req_out', 'Out2', 1, 5, 15),
		(6, 0, 'req_in', 'Other', 2, 0, 40),
		(100, 0, 'parent', 'Parent', 3, 0, 100),
		(101, 100, 'req_in', 'Buffered', 4, 10, 20)`)
	if err != nil {
		t.Fatalf("insert trace rows: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO milestone
		(ID, TaskID, Time, Kind, What)
		VALUES
		(1, 1, 12, 'queued', 'Queued')`)
	if err != nil {
		t.Fatalf("insert milestone rows: %v", err)
	}
}

func assertValues(t *testing.T, got []TimeValue, want []float64) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d values, want %d", len(got), len(want))
	}

	for i := range want {
		if math.Abs(got[i].Value-want[i]) > 1e-9 {
			t.Fatalf("value[%d] = %v, want %v", i, got[i].Value, want[i])
		}
	}
}

func TestComponentInfoBinsReqMetricsWithSQLAggregation(t *testing.T) {
	reader := newTestTraceReader(t)
	insertTraceRows(t, reader)
	server := &Server{traceReader: reader}

	reqIn := server.calculateReqIn(context.Background(), "A", 0, 40, 4)
	assertValues(t, reqIn.Data, []float64{0, 0.1, 0.1, 0.1})

	reqComplete := server.calculateReqComplete(context.Background(), "A", 0, 40, 4)
	assertValues(t, reqComplete.Data, []float64{0, 0, 0.1, 0})

	avgLatency := server.calculateAvgLatency(context.Background(), "A", 0, 40, 4)
	assertValues(t, avgLatency.Data, []float64{0, 0, 10, 0})
}

func TestComponentInfoAggregatesSubtree(t *testing.T) {
	reader := newTestTraceReader(t)
	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO location (ID, Locale) VALUES (10, 'C.in'), (11, 'C.out'), (12, 'Cousin.in')`)
	// C.in: 2 req_in, C.out: 1 req_in (all start in bin 0). Cousin.in: 2 — a
	// differently-prefixed sibling that scope "C" must NOT pull in.
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(1, 0, 'req_in', 'R', 10, 1, 5),
		(2, 0, 'req_in', 'R', 10, 2, 6),
		(3, 0, 'req_in', 'R', 11, 3, 7),
		(4, 0, 'req_in', 'R', 12, 1, 5),
		(5, 0, 'req_in', 'R', 12, 1, 5)`)
	server := &Server{traceReader: reader}

	// scope "C" aggregates C.in + C.out = 3 req_in tasks in bin 0 (3/binDuration =
	// 0.3), excluding the sibling "Cousin.in".
	agg := server.calculateReqIn(context.Background(), "C", 0, 40, 4)
	assertValues(t, agg.Data, []float64{0.3, 0, 0, 0})

	// A leaf scope still matches only itself.
	leaf := server.calculateReqIn(context.Background(), "C.in", 0, 40, 4)
	assertValues(t, leaf.Data, []float64{0.2, 0, 0, 0})
}

func TestComponentInfoUsesSweepForTimeWeightedCounts(t *testing.T) {
	reader := newTestTraceReader(t)
	insertTraceRows(t, reader)
	server := &Server{traceReader: reader}

	concurrent := server.calculateConcurrentTask(
		context.Background(), nil, "A", "ConcurrentTask", 0, 20, 4)
	assertValues(t, concurrent.Data, []float64{1, 2, 2, 1})
}

// TestBufferOccupancyFromPortTasks verifies that buffer pressure is the occupancy
// of the per-port "incoming_buffer" tasks and pending request out is the occupancy
// of "req_out" tasks, both filtered by kind and aggregated across the subtree.
func TestBufferOccupancyFromPortTasks(t *testing.T) {
	reader := newTestTraceReader(t)
	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO location (ID, Locale) VALUES (10, 'D.Top.incoming'), (11, 'D.req_out')`)
	// Two incoming-buffer tasks (buffer pressure) and one req_out task (pending
	// request out) under component D.
	exec(`INSERT INTO trace (ID, ParentID, Kind, What, Location, StartTime, EndTime) VALUES
		(200, 0, 'incoming_buffer', 'R', 10, 0, 15),
		(201, 0, 'incoming_buffer', 'R', 10, 5, 20),
		(202, 0, 'req_out', 'R', 11, 0, 10)`)
	server := &Server{traceReader: reader}

	// Incoming-buffer occupancy over [0,20) in 4 bins of width 5:
	// [0,5): t200; [5,10): t200+t201; [10,15): t200+t201; [15,20): t201.
	bufferPressure := server.calculateBufferPressure(
		context.Background(), nil, "D", "BufferPressure", 0, 20, 4)
	assertValues(t, bufferPressure.Data, []float64{1, 2, 2, 1})

	// req_out occupancy: t202 spans [0,10) only.
	pendingReqOut := server.calculatePendingReqOut(
		context.Background(), nil, "D", "PendingReqOut", 0, 20, 4)
	assertValues(t, pendingReqOut.Data, []float64{1, 1, 0, 0})

	// The kind filter holds: the req_out location carries no incoming_buffer tasks.
	leaf := server.calculateBufferPressure(
		context.Background(), nil, "D.req_out", "BufferPressure", 0, 20, 4)
	assertValues(t, leaf.Data, []float64{0, 0, 0, 0})
}

func TestListTaskIntervalsLeanFetch(t *testing.T) {
	reader := newTestTraceReader(t)
	insertTraceRows(t, reader)

	intervals := reader.listTaskIntervals(context.Background(), "A", "", 0, 20)
	if len(intervals) == 0 {
		t.Fatal("expected intervals for location A")
	}

	// Count matches the exact-location, overlapping-range set (not a subtree, and
	// only tasks that overlap [0, 20)).
	var want int
	row := reader.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM trace
			WHERE Location = (SELECT ID FROM location WHERE Locale = 'A')
				AND EndTime > 0 AND StartTime < 20`)
	if err := row.Scan(&want); err != nil {
		t.Fatalf("count: %v", err)
	}
	if len(intervals) != want {
		t.Fatalf("got %d intervals, want %d", len(intervals), want)
	}

	for _, task := range intervals {
		if task.EndTime <= 0 || task.StartTime >= 20 {
			t.Fatalf("interval does not overlap [0,20): %+v", task)
		}
		// The lean fetch leaves everything but the interval unset.
		if task.Kind != "" || task.What != "" || task.Location != "" {
			t.Fatalf("lean fetch should populate only Start/End, got %+v", task)
		}
	}
}

func TestListTasksLoadsMilestonesOnlyWhenRequested(t *testing.T) {
	reader := newTestTraceReader(t)
	insertTraceRows(t, reader)

	tasks := reader.ListTasks(context.Background(), TaskQuery{Where: "A"})
	if len(tasks) == 0 {
		t.Fatal("expected tasks")
	}
	if len(tasks[0].Steps) != 0 {
		t.Fatalf("milestones loaded without EnableMilestones: %+v", tasks[0].Steps)
	}

	tasks = reader.ListTasks(context.Background(), TaskQuery{Where: "A", EnableMilestones: true})
	if len(tasks[0].Steps) != 1 {
		t.Fatalf("expected one milestone, got %+v", tasks[0].Steps)
	}
}

func TestFormatTraceRowsCapsAtMaxRows(t *testing.T) {
	reader := newTestTraceReader(t)

	if _, err := reader.Exec(`INSERT INTO location (ID, Locale) VALUES (1, 'A')`); err != nil {
		t.Fatalf("insert location: %v", err)
	}
	// Insert more than the cap so the query's LIMIT is what bounds the output.
	insert := fmt.Sprintf(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		WITH RECURSIVE seq(n) AS (
			SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n < %d
		)
		SELECT n, 0, 'k', 'w', 1, 0, 1 FROM seq`, maxTraceContextRows+100)
	if _, err := reader.Exec(insert); err != nil {
		t.Fatalf("insert trace rows: %v", err)
	}

	out := formatTraceRows(reader, buildTraceSQL([]string{"A"}, -1, 2))

	// Lines = header marker + column header + N data rows + truncation note +
	// end marker, so data rows = total newline count - 4.
	dataRows := strings.Count(out, "\n") - 4
	if dataRows != maxTraceContextRows {
		t.Errorf("data rows = %d, want %d (cap)", dataRows, maxTraceContextRows)
	}
	if !strings.Contains(out, "truncated to the first") {
		t.Error("expected a truncation note when the cap is hit")
	}
}

func TestBuildTraceSQLOrdersBeforeLimiting(t *testing.T) {
	sql := buildTraceSQL([]string{"A"}, 0, 100)

	order := strings.Index(sql, "ORDER BY t.StartTime, t.ID")
	limit := strings.Index(sql, "LIMIT")
	if order == -1 {
		t.Fatalf("expected a stable ORDER BY in trace SQL:\n%s", sql)
	}
	if limit == -1 || order > limit {
		t.Errorf("ORDER BY must precede LIMIT so the cap keeps the earliest events:\n%s", sql)
	}
}
