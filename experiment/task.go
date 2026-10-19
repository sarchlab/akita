package experiment

import (
	"slices"
	"strings"
)

// SchemaTask is the schema of a queued task.
const SchemaTask = "exp-task/1"

// Binary is a built simulator binary, stored on hosts by content as
// bin/<sha[:12]>/<file>.
type Binary struct {
	SHA    string  `json:"sha"`
	File   string  `json:"file,omitempty"`
	Commit string  `json:"commit,omitempty"`
	Dirty  bool    `json:"dirty,omitempty"`
	Diff   string  `json:"diff,omitempty"` // hash of the uncommitted changes
	Target string  `json:"target,omitempty"`
	Built  float64 `json:"built,omitempty"`
}

// ShortSHA is the directory name of a binary in bin/.
func (b Binary) ShortSHA() string {
	if len(b.SHA) > shortSHALen {
		return b.SHA[:shortSHALen]
	}

	return b.SHA
}

// Revision is the commit, with "-dirty-<diff>" for uncommitted changes.
func (b Binary) Revision() string {
	if b.Dirty {
		return b.Commit + "-dirty-" + b.Diff
	}

	return b.Commit
}

// shortSHALen is the length of the sha prefix that names a binary.
const shortSHALen = 12

// Attempt is one run of a task on a host.
type Attempt struct {
	Host    string   `json:"host"`
	Exit    *int     `json:"exit"`
	Signal  string   `json:"signal,omitempty"`
	Started float64  `json:"started,omitempty"`
	Ended   *float64 `json:"ended,omitempty"`
}

// Task is a queued command (schema exp-task/1). The fields after Submitted
// are set by the host's agent while it runs and after it ended.
type Task struct {
	Schema      string            `json:"schema"`
	Project     string            `json:"project"`
	Name        string            `json:"name"`
	Group       string            `json:"group,omitempty"`
	Priority    int               `json:"priority"`
	MemGB       float64           `json:"mem_gb,omitempty"`
	Cmd         string            `json:"cmd"`
	Binaries    map[string]Binary `json:"binaries,omitempty"`
	Attempts    []Attempt         `json:"attempts,omitempty"`
	AvoidHosts  []string          `json:"avoid_hosts,omitempty"`
	Submitted   float64           `json:"submitted"`
	SubmittedBy string            `json:"submitted_by,omitempty"`

	Seq       int64    `json:"seq,omitempty"`
	Host      string   `json:"host,omitempty"`
	PID       int      `json:"pid,omitempty"`
	PGID      int      `json:"pgid,omitempty"`
	StartTick uint64   `json:"start_tick,omitempty"` // the leader's start time, to tell pid reuse
	Started   float64  `json:"started,omitempty"`
	Paused    bool     `json:"paused,omitempty"`
	PausedAt  float64  `json:"paused_at,omitempty"`
	State     string   `json:"state,omitempty"` // done: ok failed stopped died
	Exit      *int     `json:"exit,omitempty"`
	Signal    string   `json:"signal,omitempty"`
	Ended     float64  `json:"ended,omitempty"`
	Error     string   `json:"error,omitempty"`
	Retryable bool     `json:"retryable,omitempty"`
	Retried   string   `json:"retried,omitempty"` // host the retry went to
	Collected *float64 `json:"collected,omitempty"`
}

// Key is "<project>--<name>", the task's file name in the queue.
func (t *Task) Key() string { return TaskKey(t.Project, t.Name) }

// TaskKey is the file name of a task in the queue.
func TaskKey(project, name string) string { return project + "--" + name }

// Avoids tells whether the task must not run on a host.
func (t *Task) Avoids(host string) bool { return slices.Contains(t.AvoidHosts, host) }

// BinaryProperties are the run.json properties of the task's binaries:
// "Binary" (name and sha of each) and "Git Revision".
func (t *Task) BinaryProperties() []Property {
	if len(t.Binaries) == 0 {
		return nil
	}

	names := make([]string, 0, len(t.Binaries))
	for n := range t.Binaries {
		names = append(names, n)
	}

	slices.Sort(names)

	var bins, revs []string

	for _, n := range names {
		b := t.Binaries[n]
		bins = append(bins, n+" "+b.ShortSHA())

		if r := b.Revision(); r != "" && !slices.Contains(revs, r) {
			revs = append(revs, r)
		}
	}

	props := []Property{{Property: "Binary", Value: strings.Join(bins, ", ")}}
	if len(revs) > 0 {
		props = append(props, Property{Property: "Git Revision", Value: strings.Join(revs, ", ")})
	}

	return props
}
