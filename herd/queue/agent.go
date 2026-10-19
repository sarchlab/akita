package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// Agent timings and limits.
const (
	DefaultInterval = 5 * time.Second
	// StaleAfter is the heartbeat age after which an agent counts as gone.
	StaleAfter = 60 * time.Second
	// warmup is how long a started task's mem_gb is still subtracted from
	// MemAvailable, until the task has allocated it.
	warmup = 60 * time.Second
	// lostAfter is how long a running task may lack a pid before it is
	// given up (the agent died between moving and starting it).
	lostAfter    = 60 * time.Second
	exitWait     = 200 * time.Millisecond
	logHeadBytes = 4 << 10
	logTailBytes = 16 << 10
	maxSignal    = 64
	signalBase   = 128
	gb           = 1 << 30
)

// Defaults of the memory thresholds (GB).
const (
	defaultMemReserveGB = 4
	defaultMemPauseGB   = 2
	defaultMemResumeGB  = 6
)

// System is what the agent asks of the machine; tests replace it.
type System interface {
	MemAvailable(ctx context.Context) (uint64, error)
	// Simulators counts the processes that are simulators of the specs.
	Simulators(ctx context.Context, specs []herd.SimulatorSpec) int
	Start(dir string, argv, env []string) (int, <-chan hostsys.Exit, error)
	GroupAlive(pgid int) bool
	StartTick(pid int) uint64
	Pause(pgid int) error
	Resume(pgid int) error
	SignalName(n int) string
}

// Config is an agent's host name and settings.
type Config struct {
	Host string
	herd.QueueConfig

	Version string
}

// WithDefaults fills in the default settings.
func (c Config) WithDefaults() Config {
	if c.Slots <= 0 {
		c.Slots = max(1, runtime.NumCPU()/2) //nolint:mnd // half of the CPUs
	}

	if c.MemReserveGB <= 0 {
		c.MemReserveGB = defaultMemReserveGB
	}

	if c.MemPauseGB <= 0 {
		c.MemPauseGB = defaultMemPauseGB
	}

	if c.MemResumeGB <= 0 {
		c.MemResumeGB = max(defaultMemResumeGB, c.MemPauseGB+defaultMemPauseGB)
	}

	return c
}

// Agent runs the queue of a root: it reaps ended tasks, pauses and resumes
// tasks under memory pressure, and starts pending tasks.
type Agent struct {
	q     *Queue
	cfg   Config
	sys   System
	now   func() time.Time
	log   io.Writer
	exits map[int]<-chan hostsys.Exit
	info  herd.AgentInfo
}

// NewAgent creates the agent of a root.
func NewAgent(root string, cfg Config, sys System, log io.Writer) *Agent {
	cfg = cfg.WithDefaults()
	a := &Agent{
		q: Open(root), cfg: cfg, sys: sys, now: time.Now, log: log,
		exits: map[int]<-chan hostsys.Exit{},
	}
	a.info = herd.AgentInfo{
		PID: os.Getpid(), StartTick: hostsys.ProcStartTick(os.Getpid()), Host: cfg.Host,
		Version: cfg.Version, Slots: cfg.Slots, MemReserveGB: cfg.MemReserveGB,
		MemPauseGB: cfg.MemPauseGB, MemResumeGB: cfg.MemResumeGB, AvoidSignals: cfg.AvoidSignals,
	}

	return a
}

// Run holds the root's agent lock and ticks every interval until ctx is
// done. Running tasks are left alone when it returns.
func (a *Agent) Run(ctx context.Context, interval time.Duration) error {
	if err := a.q.init(); err != nil {
		return err
	}

	lock, err := hostsys.Flock(a.q.path(agentLock), 0)
	if errors.Is(err, hostsys.ErrLocked) {
		info, _ := a.q.Agent()

		pid := 0
		if info != nil {
			pid = info.PID
		}

		return fmt.Errorf("an agent already runs for %s (pid %d)", a.q.root, pid)
	}

	if err != nil {
		return err
	}
	defer lock.Close()

	a.info.Started = herd.UnixSeconds(a.now())
	a.logf("agent started: host %s, %d slots, memory reserve/pause/resume %g/%g/%g GB",
		a.cfg.Host, a.cfg.Slots, a.cfg.MemReserveGB, a.cfg.MemPauseGB, a.cfg.MemResumeGB)

	for {
		if err := a.Tick(ctx); err != nil {
			a.logf("tick: %v", err)
		}

		select {
		case <-ctx.Done():
			a.info.Stopped = herd.UnixSeconds(a.now())
			_ = a.writeInfo()
			a.logf("agent stopped; running tasks keep running")

			return nil
		case <-time.After(interval):
		}
	}
}

// Tick runs one round: reap, memory guard, start, heartbeat.
func (a *Agent) Tick(ctx context.Context) error {
	mem, err := a.sys.MemAvailable(ctx)
	if err != nil {
		return err
	}

	external := a.sys.Simulators(ctx, a.simulatorSpecs())

	var own int

	err = a.q.locked(func() error {
		running := a.reap(ctx)
		a.guard(running, mem)
		own = a.start(ctx, running, mem, external)

		return nil
	})

	a.info.Heartbeat = herd.UnixSeconds(a.now())
	a.info.Own, a.info.External = own, external
	a.info.MemAvailGB = float64(mem) / gb
	a.info.Error = ""

	if err != nil {
		a.info.Error = err.Error()
	}

	if werr := a.writeInfo(); err == nil {
		err = werr
	}

	return err
}

func (a *Agent) logf(format string, args ...any) {
	if a.log != nil {
		fmt.Fprintf(a.log, "%s %s\n", a.now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
	}
}

func (a *Agent) writeInfo() error {
	data, err := json.MarshalIndent(a.info, "", "  ")
	if err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(a.q.path(agentFile), append(data, '\n'), 0o644)
}

// simulatorSpecs are the simulators of the projects under the root.
func (a *Agent) simulatorSpecs() []herd.SimulatorSpec {
	entries, _ := os.ReadDir(a.q.root)

	var specs []herd.SimulatorSpec

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		if cfg, err := herd.LoadProjectConfig(filepath.Join(a.q.root, e.Name())); err == nil && cfg != nil {
			specs = append(specs, cfg.Simulators...)
		}
	}

	return specs
}

// alive tells whether a running task's process group lives and is still
// the one the agent started (its leader, if alive, has the recorded start
// time).
func (a *Agent) alive(t *herd.Task) bool {
	if t.PGID <= 1 || !a.sys.GroupAlive(t.PGID) {
		return false
	}

	tick := a.sys.StartTick(t.PID)

	return tick == 0 || t.StartTick == 0 || tick == t.StartTick
}

// reap moves the running tasks whose process group is gone to done/ and
// returns the others.
func (a *Agent) reap(ctx context.Context) []taskFile {
	var live []taskFile

	for _, f := range a.q.files(RunningDir) {
		t := f.task
		if t.PGID == 0 && !a.adopt(f) {
			if a.now().Sub(time.Unix(int64(t.Started), 0)) < lostAfter {
				live = append(live, f)
				continue
			}
		} else if a.alive(t) {
			live = append(live, f)
			continue
		}

		a.end(ctx, f, nil)
	}

	return live
}

// adopt takes the pid of a task that the agent moved to running/ but did
// not record from its .job file.
func (a *Agent) adopt(f taskFile) bool {
	t := f.task

	j := workspace.ReadJob(workspace.LogsPath(a.q.Workspace(t.Project)), t.Name)
	if j == nil || j.Kind != workspace.JobPID {
		return false
	}

	var pid int
	if _, err := fmt.Sscan(j.ID, &pid); err != nil || pid <= 1 {
		return false
	}

	t.PID, t.PGID, t.StartTick = pid, pid, a.sys.StartTick(pid)

	return writeTask(f.path, t) == nil
}

// end records how a task ended and moves it to done/. failure is set when
// it could not be started.
func (a *Agent) end(ctx context.Context, f taskFile, failure error) {
	t := f.task
	now := herd.UnixSeconds(a.now())
	t.Ended, t.Paused = now, false

	if failure != nil {
		t.State, t.Error = "failed", failure.Error()
	} else {
		a.outcome(t)
		a.record(ctx, t)
	}

	t.Retryable = t.Signal != "" && slices.Contains(a.cfg.AvoidSignals, t.Signal)
	t.Attempts = append(t.Attempts, herd.Attempt{
		Host: a.cfg.Host, Exit: t.Exit, Signal: t.Signal, Started: t.Started, Ended: &now,
	})

	if err := a.q.move(f.path, DoneDir, t); err != nil {
		a.logf("%s: %v", t.Key(), err)
		return
	}

	msg := t.State
	if t.Signal != "" {
		msg += " (" + t.Signal + ")"
	}

	if t.Retryable {
		msg += ", retryable elsewhere"
	}

	a.logf("ended %s: %s", t.Key(), msg)
}

// outcome reads the exit of an ended task from its log markers, or from
// its wait status when this agent started it.
func (a *Agent) outcome(t *herd.Task) {
	logPath := workspace.SessionFile(workspace.LogsPath(a.q.Workspace(t.Project)), t.Name, workspace.LogExt)

	st := workspace.LogState{State: "died"}
	if l, err := workspace.ReadLog(logPath, logHeadBytes, logTailBytes); err == nil {
		st = l.Markers()
	}

	t.State = st.State

	switch {
	case st.ExitCode != nil:
		t.Exit = st.ExitCode
		if c := *st.ExitCode; c > signalBase && c <= signalBase+maxSignal {
			t.Signal = a.sys.SignalName(c - signalBase)
		}
	case st.State == "died":
		if ex, ok := a.waitStatus(t.PID); ok {
			if ex.Signal != "" {
				t.Signal = ex.Signal
			} else if ex.Code >= 0 {
				t.Exit = &ex.Code
			}
		}
	}

	delete(a.exits, t.PID)
}

func (a *Agent) waitStatus(pid int) (hostsys.Exit, bool) {
	ch, ok := a.exits[pid]
	if !ok {
		return hostsys.Exit{}, false
	}

	select {
	case ex := <-ch:
		return ex, true
	case <-time.After(exitWait):
		return hostsys.Exit{}, false
	}
}

// record adds the binaries of a task to its run's run.json
// (results/<name>/), if the run exists.
func (a *Agent) record(ctx context.Context, t *herd.Task) {
	props := t.BinaryProperties()
	dir := filepath.Join(workspace.ResultsPath(a.q.Workspace(t.Project)), t.Name)

	if len(props) == 0 || !hostsys.IsDir(dir) || ctx.Err() != nil {
		return
	}

	cfg, _ := herd.LoadProjectConfig(a.q.Workspace(t.Project))

	var naming *herd.Naming
	if cfg != nil {
		naming = cfg.Naming
	}

	if _, err := herd.WriteRecord(dir, naming, props); err != nil {
		a.logf("%s: run.json: %v", t.Key(), err)
	}
}

// guard pauses the most recently started task of the lowest priority when
// MemAvailable is below mem_pause_gb, and resumes the oldest paused task
// when it is above mem_resume_gb; one task per tick.
func (a *Agent) guard(running []taskFile, mem uint64) {
	avail := float64(mem) / gb

	switch {
	case avail < a.cfg.MemPauseGB:
		var pick *taskFile

		for i := range running {
			t := running[i].task
			if t.Paused || !a.alive(t) {
				continue
			}

			if pick == nil || t.Priority > pick.task.Priority ||
				(t.Priority == pick.task.Priority && t.Started > pick.task.Started) {
				pick = &running[i]
			}
		}

		if pick != nil {
			a.setPaused(*pick, true, avail)
		}
	case avail > a.cfg.MemResumeGB:
		var pick *taskFile

		for i := range running {
			t := running[i].task
			if t.Paused && a.alive(t) && (pick == nil || t.Started < pick.task.Started) {
				pick = &running[i]
			}
		}

		if pick != nil {
			a.setPaused(*pick, false, avail)
		}
	}
}

func (a *Agent) setPaused(f taskFile, pause bool, avail float64) {
	t := f.task

	op, verb := a.sys.Resume, "resumed"
	if pause {
		op, verb = a.sys.Pause, "paused"
	}

	if err := op(t.PGID); err != nil {
		a.logf("%s %s: %v", verb, t.Key(), err)
		return
	}

	t.Paused, t.PausedAt = pause, 0
	if pause {
		t.PausedAt = herd.UnixSeconds(a.now())
	}

	if err := writeTask(f.path, t); err != nil {
		a.logf("%s: %v", t.Key(), err)
	}

	a.logf("%s %s (pgid %d): MemAvailable %.1f GB", verb, t.Key(), t.PGID, avail)
}

// start launches pending tasks while slots and memory allow, in priority
// then submission order; none while a task is paused. It returns the
// number of the agent's running tasks.
func (a *Agent) start(ctx context.Context, running []taskFile, mem uint64, external int) int {
	own := len(running)
	avail := float64(mem) / gb

	for _, f := range running {
		if f.task.Paused {
			return own
		}

		if a.now().Sub(time.Unix(int64(f.task.Started), 0)) < warmup {
			avail -= f.task.MemGB
		}
	}

	for _, f := range a.q.files(PendingDir) {
		t := f.task
		if max(own, external) >= a.cfg.Slots {
			break
		}

		if t.Avoids(a.cfg.Host) {
			continue
		}

		if avail-t.MemGB < a.cfg.MemReserveGB {
			break
		}

		if a.launch(ctx, f) {
			own++
			external++
			avail -= t.MemGB
		}
	}

	return own
}

// launch starts a pending task as a session of its workspace.
func (a *Agent) launch(ctx context.Context, f taskFile) bool {
	t := f.task
	t.Host, t.Started = a.cfg.Host, herd.UnixSeconds(a.now())
	ws := a.q.Workspace(t.Project)

	cmd, err := a.command(t)
	if err == nil {
		err = workspace.PrepareSession(ctx, ws, t.Project, t.Name, cmd, t.Group)
	}

	if err != nil {
		a.end(ctx, f, err)
		return false
	}

	// In running/ before it starts: if the agent dies now, the next one
	// adopts the task from its .job file.
	run := taskFile{path: filepath.Join(a.q.stateDir(RunningDir), t.Key()+".json"), task: t}
	if err := a.q.move(f.path, RunningDir, t); err != nil {
		a.logf("%s: %v", t.Key(), err)
		return false
	}

	env := append(os.Environ(), "HERD_TASK="+t.Key())

	pid, exit, err := a.sys.Start(ws, []string{"bash", "-lc", workspace.WrapperScript(a.cfg.Host, ws, t.Name, cmd)}, env)
	if err == nil {
		err = workspace.WriteJob(ws, t.Name, pid)
	}

	if err != nil {
		a.end(ctx, run, err)
		return false
	}

	t.PID, t.PGID, t.StartTick = pid, pid, a.sys.StartTick(pid)
	a.exits[pid] = exit

	if err := writeTask(run.path, t); err != nil {
		a.logf("%s: %v", t.Key(), err)
	}

	a.logf("started %s (pid %d)", t.Key(), pid)

	return true
}

// command is a task's command with {bin:<name>} replaced by the path of
// the binary on this machine.
func (a *Agent) command(t *herd.Task) (string, error) {
	cmd := t.Cmd

	for name, b := range t.Binaries {
		p := a.q.BinaryPath(b)
		if !hostsys.Exists(p) {
			return "", fmt.Errorf("binary %s (%s) is not on this host", name, b.ShortSHA())
		}

		cmd = strings.ReplaceAll(cmd, "{bin:"+name+"}", workspace.ShellQuote(p))
	}

	if i := strings.Index(cmd, "{bin:"); i >= 0 {
		return "", fmt.Errorf("unknown binary in %q", cmd[i:])
	}

	return cmd, nil
}

// Agent reads agent.json; alive tells whether that agent's process lives
// and its heartbeat is recent.
func (q *Queue) Agent() (*herd.AgentInfo, bool) {
	data, err := hostsys.ReadLimited(q.path(agentFile), maxTaskSize)
	if err != nil {
		return nil, false
	}

	info := &herd.AgentInfo{}
	if json.Unmarshal(data, info) != nil {
		return nil, false
	}

	age := time.Since(time.Unix(int64(info.Heartbeat), 0))
	alive := info.Stopped == 0 && hostsys.PIDAlive(info.PID) && age < StaleAfter &&
		(info.StartTick == 0 || hostsys.ProcStartTick(info.PID) == info.StartTick)

	return info, alive
}

// StopAgent asks the root's agent to stop (SIGTERM to the pid it
// recorded) and waits for it to exit.
func (q *Queue) StopAgent(timeout time.Duration) (*herd.AgentInfo, error) {
	info, _ := q.Agent()
	if info == nil || info.Stopped != 0 || !hostsys.PIDAlive(info.PID) ||
		(info.StartTick != 0 && hostsys.ProcStartTick(info.PID) != info.StartTick) {
		return info, errors.New("no agent is running")
	}

	if err := hostsys.Terminate(info.PID); err != nil {
		return info, err
	}

	deadline := time.Now().Add(timeout)
	for hostsys.PIDAlive(info.PID) {
		if time.Now().After(deadline) {
			return info, fmt.Errorf("agent pid %d did not exit", info.PID)
		}

		time.Sleep(lockPoll)
	}

	return info, nil
}

// lockPoll is how often StopAgent checks the agent.
const lockPoll = 100 * time.Millisecond

// HostSystem is the System of this machine. FakeMemFile, when set, names a
// file holding MemAvailable in GB, which replaces the real value (tests of
// the memory guard).
type HostSystem struct {
	FakeMemFile string
}

// MemAvailable reads MemAvailable.
func (s HostSystem) MemAvailable(ctx context.Context) (uint64, error) {
	if s.FakeMemFile != "" {
		var v float64
		if _, err := fmt.Sscan(hostsys.ReadText(s.FakeMemFile, 100), &v); err != nil {
			return 0, fmt.Errorf("%s: %w", s.FakeMemFile, err)
		}

		return uint64(v * gb), nil
	}

	return hostsys.MemAvailable(ctx)
}

// Simulators counts the simulator processes of this machine.
func (HostSystem) Simulators(ctx context.Context, specs []herd.SimulatorSpec) int {
	if len(specs) == 0 {
		return 0
	}

	n := 0

	for _, args := range hostsys.ProcessArgs(ctx) {
		for _, s := range specs {
			if s.Matches(args) {
				n++
				break
			}
		}
	}

	return n
}

// Start starts a process group.
func (HostSystem) Start(dir string, argv, env []string) (int, <-chan hostsys.Exit, error) {
	return hostsys.StartGroup(dir, argv, env)
}

// GroupAlive tells whether a process group has a process.
func (HostSystem) GroupAlive(pgid int) bool { return hostsys.GroupAlive(pgid) }

// StartTick is a process's start time.
func (HostSystem) StartTick(pid int) uint64 { return hostsys.ProcStartTick(pid) }

// Pause stops a process group.
func (HostSystem) Pause(pgid int) error { return hostsys.PauseGroup(pgid) }

// Resume continues a process group.
func (HostSystem) Resume(pgid int) error { return hostsys.ResumeGroup(pgid) }

// SignalName names a signal number.
func (HostSystem) SignalName(n int) string { return hostsys.SignalName(n) }
