// Package tracingtest provides tracing test helpers shared across the
// component packages, so each does not re-implement the same recording tracer.
package tracingtest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/tracing"
)

// LeakRecorder is a [tracing.Tracer] that tracks which tasks a component has
// started but not yet ended. It exists for reset-leak tests: attach it with
// [tracing.CollectTrace], drive the component into an in-flight state, issue a
// control Reset, then assert [LeakRecorder.OpenTasks] is empty — any task still
// open is a started-never-ended leak (and, for a req_in, a leaked
// receiver-registry entry, since the component ends and forgets it together).
//
// It records only task starts and ends, so milestones and tags — which never
// open a task — do not register, and a blanket end-on-reset of a task that was
// never started is a no-op (deleting an absent key). Do not attach
// [tracing.CollectIncomingBufferTrace]: the buffer tasks it opens are ended by
// the port retrieve hook, not the component, and would otherwise show up here.
type LeakRecorder struct {
	tracing.NopTracer

	started int
	open    map[uint64]tracing.TaskStart
}

// StartTask records a task as open.
func (r *LeakRecorder) StartTask(s tracing.TaskStart) {
	if r.open == nil {
		r.open = map[uint64]tracing.TaskStart{}
	}

	r.started++
	r.open[s.ID] = s
}

// EndTask removes a task from the open set. Ending a task that was never started
// is a no-op, which is what makes blanket end-on-reset safe.
func (r *LeakRecorder) EndTask(e tracing.TaskEnd) {
	delete(r.open, e.ID)
}

// NumStarted reports how many tasks were started, so a test can assert its
// traffic actually opened tasks (otherwise an empty open set is vacuous).
func (r *LeakRecorder) NumStarted() int {
	return r.started
}

// OpenTasks returns the tasks that were started but not ended, sorted by ID for
// a stable failure message.
func (r *LeakRecorder) OpenTasks() []tracing.TaskStart {
	out := make([]tracing.TaskStart, 0, len(r.open))
	for _, s := range r.open {
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// OpenSummary renders the still-open tasks as "kind:what" entries for a test
// failure message; it returns "none" when there is no leak.
func (r *LeakRecorder) OpenSummary() string {
	open := r.OpenTasks()
	if len(open) == 0 {
		return "none"
	}

	parts := make([]string, len(open))
	for i, s := range open {
		parts[i] = fmt.Sprintf("%s:%s", s.Kind, s.What)
	}

	return strings.Join(parts, ", ")
}
