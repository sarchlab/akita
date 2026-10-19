//go:build unix

package hostsys

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlockExcludes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "q.lock")

	f, err := Flock(p, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Flock(p, 100*time.Millisecond); err != ErrLocked {
		t.Fatalf("second lock: %v, want ErrLocked", err)
	}

	f.Close()

	g, err := Flock(p, time.Second)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}

	g.Close()
}

func TestStartGroupPauseResume(t *testing.T) {
	pid, done, err := StartGroup(t.TempDir(), []string{"sh", "-c", "sleep 30 & wait"}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}

	if !GroupAlive(pid) || ProcStartTick(pid) == 0 {
		t.Fatal("started group not alive")
	}

	if err := PauseGroup(pid); err != nil {
		t.Fatal(err)
	}

	if err := ResumeGroup(pid); err != nil {
		t.Fatal(err)
	}

	if err := signalGroup(pid, 9); err != nil {
		t.Fatal(err)
	}

	select {
	case ex := <-done:
		if ex.Signal != "SIGKILL" {
			t.Fatalf("exit = %+v, want SIGKILL", ex)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("group did not end")
	}

	deadline := time.Now().Add(5 * time.Second)
	for GroupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if GroupAlive(pid) {
		t.Fatal("group alive after SIGKILL")
	}
}

func TestStartGroupExitCode(t *testing.T) {
	_, done, err := StartGroup(t.TempDir(), []string{"sh", "-c", "exit 3"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if ex := <-done; ex.Code != 3 || ex.Signal != "" {
		t.Fatalf("exit = %+v", ex)
	}

	if SignalName(9) != "SIGKILL" {
		t.Fatalf("SignalName(9) = %q", SignalName(9))
	}

	if args := ProcessArgs(context.Background()); len(args[os.Getpid()]) == 0 {
		t.Fatal("ProcessArgs misses this process")
	}
}
