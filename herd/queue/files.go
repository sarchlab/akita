package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/herd"
	"github.com/sarchlab/akita/v5/herd/internal/hostsys"
	"github.com/sarchlab/akita/v5/herd/workspace"
)

// Files and directories under <root>/.akita-herd/.
const (
	agentFile   = "agent.json"
	agentLock   = "agent.lock"
	AgentLog    = "agent.log"
	queueLock   = "queue.lock"
	seqFile     = "seq"
	queueDir    = "queue"
	PendingDir  = "pending"
	RunningDir  = "running"
	DoneDir     = "done"
	maxTaskSize = 1 << 20
)

// lockTimeout bounds the wait for queue.lock.
const lockTimeout = 30 * time.Second

// maxPriority is the largest priority (pending file names have four
// digits).
const maxPriority = 9999

// recentDone is how many ended tasks a Status lists.
const recentDone = 100

// ErrNotPending is returned by Take for a task that is not pending.
var ErrNotPending = errors.New("task is not pending")

// Queue is the queue of a host root on this machine.
type Queue struct {
	root string // "~" expanded
}

// Open returns the queue of a root ("~" is expanded).
func Open(root string) *Queue { return &Queue{root: herd.ExpandHome(root)} }

// Root is the root the queue belongs to.
func (q *Queue) Root() string { return q.root }

// Workspace is the workspace of a project under the root.
func (q *Queue) Workspace(project string) string { return filepath.Join(q.root, project) }

func (q *Queue) path(parts ...string) string {
	return filepath.Join(append([]string{q.root, Dir}, parts...)...)
}

func (q *Queue) stateDir(state string) string { return q.path(queueDir, state) }

// BinaryPath is where a binary is stored on this machine.
func (q *Queue) BinaryPath(b herd.Binary) string {
	return q.path(BinDir, b.ShortSHA(), b.File)
}

// HostBinaryPath is where a binary is stored under a host's root (kept
// unexpanded, for the host to expand).
func HostBinaryPath(root string, b herd.Binary) string {
	return strings.TrimRight(root, "/") + "/" + Dir + "/" + BinDir + "/" + b.ShortSHA() + "/" + b.File
}

func (q *Queue) init() error {
	for _, s := range []string{PendingDir, RunningDir, DoneDir} {
		if err := os.MkdirAll(q.stateDir(s), 0o755); err != nil {
			return err
		}
	}

	return nil
}

// locked runs fn holding queue.lock.
func (q *Queue) locked(fn func() error) error {
	if err := q.init(); err != nil {
		return err
	}

	f, err := hostsys.Flock(q.path(queueLock), lockTimeout)
	if err != nil {
		return fmt.Errorf("%s: %w", q.path(queueLock), err)
	}
	defer f.Close()

	return fn()
}

// pendingName is the file name of a pending task: priority, then
// submission order.
func pendingName(t *herd.Task) string {
	return fmt.Sprintf("%04d-%08d-%s.json", min(max(t.Priority, 0), maxPriority), t.Seq, t.Key())
}

// pendingKey is the task key of a pending file name ("" if malformed).
func pendingKey(file string) string {
	const prefix = len("0000-00000000-")

	base, ok := strings.CutSuffix(file, ".json")
	if !ok || len(base) <= prefix || base[4] != '-' || base[13] != '-' {
		return ""
	}

	return base[prefix:]
}

// ValidateTask checks the parts of a task that name files.
func ValidateTask(t *herd.Task) error {
	if err := workspace.ValidateRunName(t.Project); err != nil {
		return fmt.Errorf("project: %w", err)
	}

	if err := workspace.ValidateSessionName(t.Name); err != nil {
		return err
	}

	if strings.TrimSpace(t.Cmd) == "" {
		return fmt.Errorf("task %s: empty command", t.Name)
	}

	return nil
}

// Put adds tasks to pending/. It fails, adding none, if a task is invalid
// or already pending or running; an ended task of the same name is
// replaced.
func (q *Queue) Put(tasks []*herd.Task) error {
	for _, t := range tasks {
		if err := ValidateTask(t); err != nil {
			return err
		}
	}

	return q.locked(func() error {
		busy := map[string]bool{}
		for _, f := range append(q.files(PendingDir), q.files(RunningDir)...) {
			busy[f.task.Key()] = true
		}

		for _, t := range tasks {
			if busy[t.Key()] {
				return fmt.Errorf("task %s of %s is already queued or running on this host", t.Name, t.Project)
			}

			busy[t.Key()] = true
		}

		for _, t := range tasks {
			if err := q.putOne(t); err != nil {
				return err
			}
		}

		return nil
	})
}

func (q *Queue) putOne(t *herd.Task) error {
	seq, err := q.nextSeq()
	if err != nil {
		return err
	}

	t.Schema, t.Seq = herd.SchemaTask, seq
	clearRun(t)

	if err := writeTask(filepath.Join(q.stateDir(PendingDir), pendingName(t)), t); err != nil {
		return err
	}

	err = os.Remove(filepath.Join(q.stateDir(DoneDir), t.Key()+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	return err
}

// clearRun forgets the fields of a previous run of a task.
func clearRun(t *herd.Task) {
	keep := *t
	*t = herd.Task{
		Schema: keep.Schema, Project: keep.Project, Name: keep.Name, Group: keep.Group,
		Priority: keep.Priority, MemGB: keep.MemGB, Cmd: keep.Cmd, Binaries: keep.Binaries,
		Attempts: keep.Attempts, AvoidHosts: keep.AvoidHosts, Submitted: keep.Submitted,
		SubmittedBy: keep.SubmittedBy, Seq: keep.Seq,
	}
}

func (q *Queue) nextSeq() (int64, error) {
	p := q.path(seqFile)
	n, _ := strconv.ParseInt(strings.TrimSpace(hostsys.ReadText(p, 100)), 10, 64)
	n++

	return n, hostsys.WriteFileAtomic(p, []byte(strconv.FormatInt(n, 10)+"\n"), 0o644)
}

// Take removes a pending task and returns it; ErrNotPending if it is not
// pending (for example because the agent started it).
func (q *Queue) Take(project, name string) (*herd.Task, error) {
	var out *herd.Task

	err := q.locked(func() error {
		file, t := q.findPending(herd.TaskKey(project, name))
		if t == nil {
			return fmt.Errorf("%s: %w", name, ErrNotPending)
		}

		out = t

		return os.Remove(file)
	})

	return out, err
}

func (q *Queue) findPending(key string) (string, *herd.Task) {
	entries, _ := os.ReadDir(q.stateDir(PendingDir))
	for _, e := range entries {
		if pendingKey(e.Name()) != key {
			continue
		}

		p := filepath.Join(q.stateDir(PendingDir), e.Name())
		if t, err := readTask(p); err == nil {
			return p, t
		}
	}

	return "", nil
}

// Update changes an ended task (e.g. marks it collected or retried).
func (q *Queue) Update(project, name string, fn func(*herd.Task)) error {
	return q.locked(func() error {
		p := filepath.Join(q.stateDir(DoneDir), herd.TaskKey(project, name)+".json")

		t, err := readTask(p)
		if err != nil {
			return err
		}

		fn(t)

		return writeTask(p, t)
	})
}

// taskFile is a task and its file.
type taskFile struct {
	path string
	task *herd.Task
}

func (q *Queue) files(state string) []taskFile {
	entries, _ := os.ReadDir(q.stateDir(state))

	var out []taskFile

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		p := filepath.Join(q.stateDir(state), e.Name())
		if t, err := readTask(p); err == nil {
			out = append(out, taskFile{path: p, task: t})
		}
	}

	return out
}

// Status reads the queue without taking the lock: renames are atomic, so
// a task is seen in one state (or, while it moves, in none).
func (q *Queue) Status(now time.Time) *herd.QueueStatus {
	s := &herd.QueueStatus{Root: q.root, Now: herd.UnixSeconds(now)}
	s.Agent, s.Alive = q.Agent()

	for _, st := range []struct {
		dir string
		out *[]herd.Task
	}{{PendingDir, &s.Pending}, {RunningDir, &s.Running}, {DoneDir, &s.Done}} {
		for _, f := range q.files(st.dir) {
			*st.out = append(*st.out, *f.task)
		}
	}

	slices.SortStableFunc(s.Done, func(a, b herd.Task) int { return compareFloat(b.Ended, a.Ended) })

	if len(s.Done) > recentDone {
		s.Done = s.Done[:recentDone]
	}

	return s
}

// Exists tells whether the root has a queue.
func (q *Queue) Exists() bool { return hostsys.IsDir(q.path(queueDir)) }

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}

	return 0
}

func readTask(p string) (*herd.Task, error) {
	data, err := hostsys.ReadLimited(p, maxTaskSize)
	if err != nil {
		return nil, err
	}

	t := &herd.Task{}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}

	t.Schema = herd.NormalizeSchema(t.Schema)

	return t, nil
}

func writeTask(p string, t *herd.Task) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	return hostsys.WriteFileAtomic(p, append(data, '\n'), 0o644)
}

// move renames a task file into another state and rewrites it there.
func (q *Queue) move(from string, state string, t *herd.Task) error {
	to := filepath.Join(q.stateDir(state), t.Key()+".json")
	if err := os.Rename(from, to); err != nil {
		return err
	}

	return writeTask(to, t)
}
