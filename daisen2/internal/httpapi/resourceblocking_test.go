package httpapi

import (
	"context"
	"path/filepath"
	"testing"
)

// TestResourceBlockingOccupancy verifies the per-bin count of tasks blocked on one
// resource: each task's blocked interval is the span ending at its R1 milestone
// (from the previous milestone, or the task's start), and only that resource's
// intervals count.
func TestResourceBlockingOccupancy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()
	defer reader.Close()

	exec := func(q string) {
		if _, err := reader.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`CREATE TABLE milestone (ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	exec(`INSERT INTO trace VALUES
		(1, 0, 'req_in', 'R', 1, 0, 20),
		(2, 0, 'req_in', 'R', 1, 0, 20),
		(3, 0, 'req_in', 'R', 1, 0, 20)`)
	// Task 1: blocked on R1 over [0,10]. Task 2: a prior milestone at 5, then R1 at
	// 15 → blocked on R1 over [5,15]. Task 3: blocked on R2 (must be excluded).
	exec(`INSERT INTO milestone VALUES
		(1, 1, 10, 'hardware_resource', 'R1'),
		(2, 2, 5,  'data',              'x'),
		(3, 2, 15, 'hardware_resource', 'R1'),
		(4, 3, 12, 'hardware_resource', 'R2')`)

	resp := reader.ResourceBlockingOccupancy(context.Background(), "R1", 0, 20, 2, 1)

	if resp.Total != 2 {
		t.Errorf("Total = %d, want 2 (tasks 1 and 2 block on R1)", resp.Total)
	}
	// Bin 0 = [0,10): task 1 and task 2 both blocked → 2. Bin 1 = [10,20): only
	// task 2 still blocked (until 15) → 1.
	if len(resp.Bins) != 2 || resp.Bins[0] != 2 || resp.Bins[1] != 1 {
		t.Fatalf("Bins = %v, want [2 1]", resp.Bins)
	}
}

// TestTasksBlockingOn verifies the per-task hydration: only the tasks that block
// on the resource come back, with their milestones loaded (via the new IDs query
// filter), and a task blocked on a different resource is excluded.
func TestTasksBlockingOn(t *testing.T) {
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
	exec(`INSERT INTO location VALUES (1, 'L1'), (2, 'L2')`)
	exec(`CREATE TABLE trace (
		ID INTEGER, ParentID INTEGER, Kind TEXT, What TEXT,
		Location INTEGER, StartTime REAL, EndTime REAL)`)
	exec(`CREATE TABLE milestone (ID INTEGER, TaskID INTEGER, Time REAL, Kind TEXT, What TEXT)`)
	exec(`INSERT INTO trace VALUES
		(1, 0, 'req_in', 'A', 1, 0, 100),
		(2, 0, 'req_in', 'B', 1, 0, 100),
		(3, 0, 'req_in', 'C', 2, 0, 100)`)
	exec(`INSERT INTO milestone VALUES
		(1, 1, 10, 'hardware_resource', 'R1'),
		(2, 2, 20, 'hardware_resource', 'R1'),
		(3, 3, 30, 'hardware_resource', 'R2')`)

	tasks := reader.TasksBlockingOn(context.Background(), "R1", 0, 100, 10)

	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks blocked on R1, got %d: %+v", len(tasks), tasks)
	}
	ids := map[uint64]bool{tasks[0].ID: true, tasks[1].ID: true}
	if !ids[1] || !ids[2] || ids[3] {
		t.Fatalf("expected tasks {1,2}, got %v", ids)
	}
	for _, task := range tasks {
		if len(task.Steps) == 0 {
			t.Errorf("task %d has no milestones loaded", task.ID)
		}
	}
}
