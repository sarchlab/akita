package httpapi

import (
	"context"
	"math"
	"path/filepath"
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

	_, err = reader.Exec(`CREATE TABLE milestone (
		ID INTEGER,
		TaskID INTEGER,
		Time REAL,
		Kind TEXT,
		What TEXT,
		Location TEXT
	)`)
	if err != nil {
		t.Fatalf("create milestone table: %v", err)
	}

	return reader
}

func insertTraceRows(t *testing.T, reader *SQLiteTraceReader) {
	t.Helper()

	_, err := reader.Exec(`INSERT INTO trace
		(ID, ParentID, Kind, What, Location, StartTime, EndTime)
		VALUES
		(1, 0, 'req_in', 'Req1', 'A', 10, 20),
		(2, 0, 'req_in', 'Req2', 'A', 20, 40),
		(3, 0, 'req_in', 'Req3', 'A', 35, 45),
		(4, 0, 'req_out', 'Out1', 'A', 0, 10),
		(5, 0, 'req_out', 'Out2', 'A', 5, 15),
		(6, 0, 'req_in', 'Other', 'B', 0, 40),
		(100, 0, 'parent', 'Parent', 'P', 0, 100),
		(101, 100, 'req_in', 'Buffered', 'C', 10, 20)`)
	if err != nil {
		t.Fatalf("insert trace rows: %v", err)
	}

	_, err = reader.Exec(`INSERT INTO milestone
		(ID, TaskID, Time, Kind, What, Location)
		VALUES
		(1, 1, 12, 'queued', 'Queued', 'A')`)
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

func TestComponentInfoUsesSweepForTimeWeightedCounts(t *testing.T) {
	reader := newTestTraceReader(t)
	insertTraceRows(t, reader)
	server := &Server{traceReader: reader}

	concurrent := server.calculateConcurrentTask(
		context.Background(), nil, "A", "ConcurrentTask", 0, 20, 4)
	assertValues(t, concurrent.Data, []float64{1, 2, 2, 1})

	pendingReqOut := server.calculatePendingReqOut(
		context.Background(), nil, "A", "PendingReqOut", 0, 20, 4)
	assertValues(t, pendingReqOut.Data, []float64{1, 2, 1, 0})

	bufferPressure := server.calculateBufferPressure(
		context.Background(), nil, "C", "BufferPressure", 0, 20, 4)
	assertValues(t, bufferPressure.Data, []float64{1, 1, 0, 0})
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
