//go:build unix

package queue

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/experiment"
	"github.com/sarchlab/akita/v5/experiment/internal/hostsys"
	"github.com/sarchlab/akita/v5/experiment/internal/testutil"
	"github.com/sarchlab/akita/v5/experiment/workspace"
)

// fakeSystem is a process table and a MemAvailable the test controls.
type fakeSystem struct {
	memGB   float64
	sims    int
	nextPID int
	groups  map[int]bool // alive process groups
	ticks   map[int]uint64
	started [][]string
	signals []string
}

func newFake() *fakeSystem {
	return &fakeSystem{memGB: 100, nextPID: 1000, groups: map[int]bool{}, ticks: map[int]uint64{}}
}

func (f *fakeSystem) MemAvailable(context.Context) (uint64, error) { return uint64(f.memGB * gb), nil }

func (f *fakeSystem) Simulators(context.Context, []experiment.SimulatorSpec) int { return f.sims }

func (f *fakeSystem) Start(_ string, argv, _ []string) (int, <-chan hostsys.Exit, error) {
	f.nextPID++
	f.groups[f.nextPID] = true
	f.ticks[f.nextPID] = uint64(f.nextPID * 10)
	f.started = append(f.started, argv)

	return f.nextPID, make(chan hostsys.Exit, 1), nil
}

func (f *fakeSystem) GroupAlive(pgid int) bool { return f.groups[pgid] }
func (f *fakeSystem) StartTick(pid int) uint64 { return f.ticks[pid] }

func (f *fakeSystem) Pause(pgid int) error {
	f.signals = append(f.signals, "STOP "+itoa(pgid))
	return nil
}

func (f *fakeSystem) Resume(pgid int) error {
	f.signals = append(f.signals, "CONT "+itoa(pgid))
	return nil
}

func (f *fakeSystem) SignalName(n int) string {
	return map[int]string{7: "SIGBUS", 9: "SIGKILL", 11: "SIGSEGV"}[n]
}

func itoa(n int) string { return strconv.Itoa(n) }

// stopGroup kills a process group and waits until it is gone.
func stopGroup(t *testing.T, pgid int) {
	t.Helper()

	_ = syscall.Kill(-pgid, syscall.SIGKILL)

	deadline := time.Now().Add(5 * time.Second)
	for hostsys.GroupAlive(pgid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
}

type agentEnv struct {
	t     *testing.T
	sys   *fakeSystem
	agent *Agent
	clock time.Time
}

func newAgentEnv(t *testing.T, cfg experiment.QueueConfig) *agentEnv {
	t.Helper()

	e := &agentEnv{t: t, sys: newFake(), clock: time.Unix(1_800_000_000, 0)}
	e.agent = NewAgent(t.TempDir(), Config{Host: "h1", QueueConfig: cfg}, e.sys, nil)
	e.agent.now = func() time.Time { return e.clock }

	return e
}

func (e *agentEnv) put(tasks ...*experiment.Task) {
	e.t.Helper()

	if err := e.agent.q.Put(tasks); err != nil {
		e.t.Fatal(err)
	}
}

func (e *agentEnv) tick() *experiment.QueueStatus {
	e.t.Helper()

	if err := e.agent.Tick(context.Background()); err != nil {
		e.t.Fatal(err)
	}

	e.clock = e.clock.Add(DefaultInterval)

	return e.agent.q.Status(e.clock)
}

// finish ends a running task as if its command exited with code.
func (e *agentEnv) finish(name string, code int) {
	e.t.Helper()

	for _, f := range e.agent.q.files(RunningDir) {
		if f.task.Name != name {
			continue
		}

		logPath := workspace.SessionFile(workspace.LogsPath(e.agent.q.Workspace("p")), name, workspace.LogExt)
		testutil.WriteFile(e.t, logPath, "[start] Fri Oct 16 01:56:33 AM UTC 2026  h1  \n"+
			"[exit "+itoa(code)+"] Fri Oct 16 01:58:10 AM UTC 2026\n")
		e.sys.groups[f.task.PGID] = false

		return
	}

	e.t.Fatalf("%s is not running", name)
}

func names(tasks []experiment.Task) string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		n := t.Name
		if t.Paused {
			n += "(paused)"
		}

		out = append(out, n)
	}

	return strings.Join(out, ",")
}

func TestAgentSlotsAndExternal(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 2})
	e.put(task("a", 5), task("b", 5), task("c", 5))

	s := e.tick()
	if names(s.Running) != "a,b" || names(s.Pending) != "c" {
		t.Fatalf("running %s pending %s", names(s.Running), names(s.Pending))
	}

	e.finish("a", 0)
	e.sys.sims = 2 // simulators started outside the queue hold the slots

	if s = e.tick(); names(s.Running) != "b" || names(s.Done) != "a" || s.Done[0].State != "ok" {
		t.Fatalf("running %s done %+v", names(s.Running), s.Done)
	}

	e.sys.sims = 0
	if s = e.tick(); names(s.Running) != "b,c" {
		t.Fatalf("running %s", names(s.Running))
	}

	if s.Agent == nil || s.Agent.Own != 2 || s.Agent.Slots != 2 || s.Agent.Host != "h1" {
		t.Fatalf("agent.json = %+v", s.Agent)
	}
}

func TestAgentMemoryReserveAndAvoid(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 8, MemReserveGB: 4})
	e.sys.memGB = 20

	big, avoid, small := task("big", 1), task("avoid", 0), task("small", 2)
	big.MemGB, small.MemGB = 10, 1
	avoid.AvoidHosts = []string{"h1"}
	e.put(big, avoid, small)

	// big fits (20-10 >= 4), then small (10-1 >= 4); avoid never starts here.
	s := e.tick()
	if names(s.Running) != "big,small" || names(s.Pending) != "avoid" {
		t.Fatalf("running %s pending %s", names(s.Running), names(s.Pending))
	}

	huge := task("huge", 0)
	huge.MemGB = 12
	e.put(huge)

	if s = e.tick(); names(s.Pending) != "avoid,huge" {
		t.Fatalf("huge started within the reserve: running %s", names(s.Running))
	}
}

func TestAgentMemoryGuard(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 8, MemPauseGB: 8, MemResumeGB: 24, MemReserveGB: 1})
	e.put(task("old-low", 9))
	e.tick()
	e.put(task("new-high", 1), task("new-low", 9))
	e.tick()
	e.tick()

	e.sys.memGB = 5
	if s := e.tick(); names(s.Running) != "new-high,new-low(paused),old-low" {
		t.Fatalf("after one pause: %s", names(s.Running))
	}

	if s := e.tick(); names(s.Running) != "new-high,new-low(paused),old-low(paused)" {
		t.Fatalf("after two pauses: %s", names(s.Running))
	}

	e.put(task("waiting", 0))
	e.sys.memGB = 15 // between the thresholds: nothing changes, nothing starts

	if s := e.tick(); names(s.Pending) != "waiting" || strings.Count(names(s.Running), "paused") != 2 {
		t.Fatalf("between thresholds: running %s pending %s", names(s.Running), names(s.Pending))
	}

	e.sys.memGB = 30
	if s := e.tick(); names(s.Running) != "new-high,new-low(paused),old-low" {
		t.Fatalf("after one resume: %s", names(s.Running))
	}

	want := []string{"STOP 1003", "STOP 1001", "CONT 1001"}
	if strings.Join(e.sys.signals, ";") != strings.Join(want, ";") {
		t.Fatalf("signals = %v, want %v", e.sys.signals, want)
	}
}

func TestAgentReapSignalRetryable(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 4, AvoidSignals: []string{"SIGBUS"}})
	e.put(task("bus", 0), task("segv", 0))
	e.tick()
	e.finish("bus", 128+7)
	e.finish("segv", 128+11)

	s := e.tick()
	if len(s.Done) != 2 {
		t.Fatalf("done = %+v", s.Done)
	}

	for _, d := range s.Done {
		wantRetry := d.Name == "bus"
		if d.State != "failed" || d.Retryable != wantRetry || len(d.Attempts) != 1 ||
			d.Attempts[0].Host != "h1" || d.Attempts[0].Signal != d.Signal {
			t.Errorf("%s: %+v", d.Name, d)
		}
	}
}

func TestAgentAdoptsAndChecksIdentity(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 4})
	e.put(task("a", 0), task("b", 0))
	e.tick()

	// A new agent (same root) finds both running; b's pid was reused by
	// another process (different start time), so b counts as ended.
	next := NewAgent(e.agent.q.Root(), Config{Host: "h1"}, e.sys, nil)
	next.now = e.agent.now
	e.sys.ticks[1002] = 1

	if err := next.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}

	s := e.agent.q.Status(e.clock)
	if names(s.Running) != "a" || names(s.Done) != "b" || s.Done[0].State != "died" {
		t.Fatalf("running %s done %+v", names(s.Running), s.Done)
	}
}

func TestAgentAdoptsFromJobFile(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 4})

	// The previous agent moved the task to running/ and started it, but
	// died before recording the pid.
	tk := task("a", 0)
	tk.Started = experiment.UnixSeconds(e.clock)

	if err := e.agent.q.init(); err != nil {
		t.Fatal(err)
	}

	if err := writeTask(filepath.Join(e.agent.q.stateDir(RunningDir), "p--a.json"), tk); err != nil {
		t.Fatal(err)
	}

	ws := e.agent.q.Workspace("p")
	testutil.MustMkdir(t, workspace.LogsPath(ws))

	if err := workspace.WriteJob(ws, "a", 4242); err != nil {
		t.Fatal(err)
	}

	e.sys.groups[4242], e.sys.ticks[4242] = true, 7

	s := e.tick()
	if len(s.Running) != 1 || s.Running[0].PGID != 4242 || s.Running[0].StartTick != 7 {
		t.Fatalf("running = %+v", s.Running)
	}
}

func TestAgentBinariesAndRecord(t *testing.T) {
	e := newAgentEnv(t, experiment.QueueConfig{Slots: 4})
	bin := experiment.Binary{SHA: strings.Repeat("ab", 32), File: "sim", Commit: "a41f7e2"}
	testutil.WriteFile(t, e.agent.q.BinaryPath(bin), "#!/bin/sh\n")

	ok := task("run1", 0)
	ok.Cmd, ok.Binaries = "{bin:sim} -x", map[string]experiment.Binary{"sim": bin}
	missing := task("run2", 0)
	missing.Cmd = "{bin:other}"
	e.put(ok, missing)

	s := e.tick()
	if names(s.Running) != "run1" || names(s.Done) != "run2" || !strings.Contains(s.Done[0].Error, "other") {
		t.Fatalf("running %s done %+v", names(s.Running), s.Done)
	}

	script := e.sys.started[0][2]
	if !strings.Contains(script, e.agent.q.BinaryPath(bin)) {
		t.Fatalf("wrapper does not name the binary path:\n%s", script)
	}

	ws := e.agent.q.Workspace("p")
	if got := testutil.ReadFile(t, filepath.Join(ws, "logs", "run1.job")); got != "1001\npid\n" {
		t.Fatalf(".job = %q", got)
	}

	testutil.MustMkdir(t, filepath.Join(ws, "results", "run1"))
	e.finish("run1", 0)
	e.tick()

	rec, err := experiment.ReadRecord(filepath.Join(ws, "results", "run1"))
	if err != nil {
		t.Fatal(err)
	}

	if rec.Property("Binary") != "sim abababababab" || rec.Property("Git Revision") != "a41f7e2" {
		t.Fatalf("run.json properties = %+v", rec.Properties)
	}
}

func TestAgentRealProcesses(t *testing.T) {
	root := t.TempDir()
	testutil.MustMkdir(t, filepath.Join(root, "p"))

	a := NewAgent(root, Config{Host: "h1", QueueConfig: experiment.QueueConfig{
		Slots: 4, AvoidSignals: []string{"SIGBUS"}, MemReserveGB: 0.001, MemPauseGB: 0.0005, MemResumeGB: 0.001,
	}}, HostSystem{}, nil)

	if err := a.q.Put([]*experiment.Task{
		{Project: "p", Name: "ok", Cmd: "true"},
		{Project: "p", Name: "bus", Cmd: "sh -c 'kill -BUS $$'"},
		{Project: "p", Name: "sleep", Cmd: "sleep 30"},
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	deadline := time.Now().Add(15 * time.Second)

	var s *experiment.QueueStatus

	for time.Now().Before(deadline) {
		if err := a.Tick(ctx); err != nil {
			t.Fatal(err)
		}

		if s = a.q.Status(time.Now()); len(s.Done) == 2 {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	done := map[string]experiment.Task{}
	for _, d := range s.Done {
		done[d.Name] = d
	}

	if done["ok"].State != "ok" || done["bus"].Signal != "SIGBUS" || !done["bus"].Retryable {
		t.Fatalf("done = %+v", s.Done)
	}

	if len(s.Running) != 1 || !hostsys.GroupAlive(s.Running[0].PGID) {
		t.Fatalf("running = %+v", s.Running)
	}

	pgid := s.Running[0].PGID
	t.Cleanup(func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })

	if err := hostsys.PauseGroup(pgid); err != nil {
		t.Fatal(err)
	}

	_ = hostsys.ResumeGroup(pgid)
	stopGroup(t, pgid)

	if err := a.Tick(ctx); err != nil {
		t.Fatal(err)
	}

	if s = a.q.Status(time.Now()); len(s.Running) != 0 || len(s.Done) != 3 {
		t.Fatalf("after kill: running %+v", s.Running)
	}

	if _, err := os.Stat(filepath.Join(root, "p", "logs", "sleep.log")); err != nil {
		t.Fatal(err)
	}
}
