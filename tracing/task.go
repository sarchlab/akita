package tracing

import (
	"github.com/sarchlab/akita/v5/timing"
)

// TaskStart carries the information needed to begin a task. Callers pass it to
// [StartTask]. Time is not set by the caller: [StartTask] stamps it from the
// domain clock, after the NumHooks==0 guard, so the clock is never read when
// tracing is disabled.
type TaskStart struct {
	ID       uint64
	ParentID uint64
	Kind     string
	What     string
	Location string // optional; when empty, derived by singleKindLocation
	Time     timing.VTimeInPicoSec
	Detail   any
}

// TaskEnd marks the completion of a task. Callers pass it to [EndTask].
type TaskEnd struct {
	ID   uint64
	Time timing.VTimeInPicoSec
}

// PipelineTaskKind is the Kind of a task that records a request's traversal of a
// component-internal latency pipeline. A pipelined component opens it as a
// subtask of its req_in task at pipeline entry (the same tick req_in opens, at
// retrieve) and closes it at pipeline exit. This attributes the pipeline latency
// that would otherwise be an unaccounted gap between the buffer task (which ends
// at retrieve) and the post-pipeline processing milestones on req_in.
//
// A pipeline subtask's What is "<component>.<stage>" (e.g. "L2Cache.bank"); that
// already-qualified name becomes its location, so each stage is its own
// single-kind row (see "One location, one kind" in README.md).
const PipelineTaskKind = "pipeline"

// ReqInTaskKind is the Kind of the receiver-side task that spans a component's
// handling of an incoming request, from admission to completion. It is opened by
// [TraceReqReceive] and located at "<component>.req_in".
const ReqInTaskKind = "req_in"

// ReqOutTaskKind is the Kind of the sender-side task that spans a request a
// component has issued, from send until the response arrives. It is opened by
// [TraceReqInitiate] and located at "<component>.req_out".
const ReqOutTaskKind = "req_out"

// A TaskTag is a categorical label attached to a task while it is processed,
// for example "read-hit" or "write-miss". Tags inherit their location from the
// owning task.
type TaskTag struct {
	ID     uint64                `json:"id"`
	TaskID uint64                `json:"task_id"`
	What   string                `json:"what"`
	Time   timing.VTimeInPicoSec `json:"time"`
}

// MilestoneKind categorizes the blocking condition a milestone resolves.
type MilestoneKind string

const (
	MilestoneKindHardwareResource MilestoneKind = "hardware_resource"
	MilestoneKindNetworkTransfer  MilestoneKind = "network_transfer"
	MilestoneKindNetworkBusy      MilestoneKind = "network_busy"
	MilestoneKindQueue            MilestoneKind = "queue"
	MilestoneKindData             MilestoneKind = "data"
	MilestoneKindDependency       MilestoneKind = "dependency"
	MilestoneKindOther            MilestoneKind = "other"
	MilestoneKindTranslation      MilestoneKind = "translation"
	// MilestoneKindSubTask marks a wait on a child subtask. Like
	// MilestoneKindWork it asserts internal activity, so it requires a
	// corresponding child subtask to exist (see "Coverage principles" in the
	// package README).
	MilestoneKindSubTask MilestoneKind = "subtask"
	// MilestoneKindWork marks the end of an interval the component spent doing
	// productive work rather than blocked on a resource — e.g. traversing an
	// internal latency pipeline. The interval from the previous milestone (or
	// task start) to a work milestone is time the task was working, not waiting.
	//
	// Coverage principle: a work milestone must be paired with a child subtask
	// (parented to the req_in, e.g. PipelineTaskKind) spanning the same interval,
	// so the trace shows what the work was instead of leaving an unexplained gap.
	// A bare work milestone with no subtask is a convention violation. See
	// "Coverage principles" in the package README.
	MilestoneKindWork MilestoneKind = "work"
)

// Milestone represents a point in time where a task's blocking status is
// resolved. Its location is inherited from the owning task.
type Milestone struct {
	ID     uint64                `json:"id"`
	TaskID uint64                `json:"task_id"`
	Time   timing.VTimeInPicoSec `json:"time"`
	Kind   MilestoneKind         `json:"kind"`
	What   string                `json:"what"`
}

// A Task is the aggregate record that a stateful tracer builds from the stream
// of trace events.
type Task struct {
	ID         uint64                `json:"id"`
	ParentID   uint64                `json:"parent_id"`
	Kind       string                `json:"kind"`
	What       string                `json:"what"`
	Location   string                `json:"location"`
	StartTime  timing.VTimeInPicoSec `json:"start_time"`
	EndTime    timing.VTimeInPicoSec `json:"end_time"`
	Tags       []TaskTag             `json:"tags"`
	Milestones []Milestone           `json:"milestones"`
	Detail     any                   `json:"-"`
	ParentTask *Task                 `json:"-"`
}

// TaskFilter decides whether a task is interesting. It is evaluated when the
// task starts. Returning true means the task is useful.
type TaskFilter func(t TaskStart) bool
