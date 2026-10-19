package queue

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/herd"
)

func task(name string, prio int) *herd.Task {
	return &herd.Task{Project: "p", Name: name, Priority: prio, Cmd: "true"}
}

func TestPutOrderAndTake(t *testing.T) {
	q := Open(t.TempDir())

	if err := q.Put([]*herd.Task{task("late", 5), task("urgent", 1), task("later", 5)}); err != nil {
		t.Fatal(err)
	}

	s := q.Status(time.Now())

	names := make([]string, 0, len(s.Pending))
	for _, p := range s.Pending {
		names = append(names, p.Name)
	}

	if len(names) != 3 || names[0] != "urgent" || names[1] != "late" || names[2] != "later" {
		t.Fatalf("pending order = %v", names)
	}

	if err := q.Put([]*herd.Task{task("late", 1)}); err == nil {
		t.Fatal("a duplicate pending task was accepted")
	}

	got, err := q.Take("p", "late")
	if err != nil || got.Name != "late" || got.Seq == 0 {
		t.Fatalf("Take = %+v, %v", got, err)
	}

	if _, err := q.Take("p", "late"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("second Take: %v, want ErrNotPending", err)
	}

	if err := q.Put([]*herd.Task{{Project: "p", Name: "../x", Cmd: "true"}}); err == nil {
		t.Fatal("a bad name was accepted")
	}
}

func TestPendingKey(t *testing.T) {
	cases := map[string]string{
		"0005-00000001-p--a.json":   "p--a",
		"0005-00000001-x-p--a.json": "x-p--a",
		"bad.json":                  "",
		"0005-00000001-.json":       "",
	}
	for in, want := range cases {
		if got := pendingKey(in); got != want {
			t.Errorf("pendingKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTakeRace(t *testing.T) {
	q := Open(t.TempDir())
	if err := q.Put([]*herd.Task{task("only", 0)}); err != nil {
		t.Fatal(err)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)

	for range 8 {
		wg.Go(func() {
			if _, err := Open(q.Root()).Take("p", "only"); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	if wins != 1 {
		t.Fatalf("%d takers got the task, want 1", wins)
	}
}

func TestPutReplacesDone(t *testing.T) {
	q := Open(t.TempDir())
	if err := q.init(); err != nil {
		t.Fatal(err)
	}

	old := task("a", 0)
	old.State, old.Exit = "failed", new(1)

	if err := writeTask(q.stateDir(DoneDir)+"/p--a.json", old); err != nil {
		t.Fatal(err)
	}

	if err := q.Put([]*herd.Task{task("a", 0)}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(q.stateDir(DoneDir) + "/p--a.json"); !os.IsNotExist(err) {
		t.Fatal("the ended task was kept")
	}

	if s := q.Status(time.Now()); len(s.Pending) != 1 || s.Pending[0].State != "" {
		t.Fatalf("pending = %+v", s.Pending)
	}
}

func TestReadTaskLegacySchema(t *testing.T) {
	p := t.TempDir() + "/t.json"
	legacy := `{"schema": "exp-task/1", "project": "p", "name": "n", "cmd": "true"}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	tk, err := readTask(p)
	if err != nil || tk.Schema != herd.SchemaTask {
		t.Errorf("got %+v, %v", tk, err)
	}
}
