// Package taskcore implements simulation tasks and framework wiring.
// Public task types are aliases in sim/tasks; framework operations stay internal.
package taskcore

import "github.com/sarchlab/akita/v5/sim/timing"

// StartInfo describes work to begin. Start allocates the ID and stamps the time.
type StartInfo struct {
	MsgID    uint64
	ParentID uint64
	Kind     string
	What     string
	Location string // optional; derived from the owner and kind when empty
	Detail   any
}

// EndInfo identifies work to complete. End stamps the time from the owner.
type EndInfo struct{ ID uint64 }

// Task is the complete basic record delivered at start and end. EndTime is
// populated at end. Tags and milestones are separate events, not replayed history.
// Detail is borrowed read-only data; observers must not mutate it. It is not
// checkpointed and is nil for restored tasks.
type Task struct {
	ID        uint64                `json:"id"`
	MsgID     uint64                `json:"msg_id"`
	ParentID  uint64                `json:"parent_id"`
	Kind      string                `json:"kind"`
	What      string                `json:"what"`
	Location  string                `json:"location"`
	StartTime timing.VTimeInPicoSec `json:"start_time"`
	EndTime   timing.VTimeInPicoSec `json:"end_time"`
	Detail    any                   `json:"-"`
}

// Shared task kinds keep trace consumers independent of component packages.
const (
	KindReqIn          = "req_in"
	KindReqOut         = "req_out"
	KindIncomingBuffer = "incoming_buffer"
	KindOutgoingBuffer = "outgoing_buffer"
	KindPipeline       = "pipeline"
	KindMsgTransport   = "msg_e2e"
	KindFlitTransport  = "flit_e2e"
)

// A Tag is a categorical label attached to a task while it is processed,
// for example "read-hit" or "write-miss". Tags inherit their location from the
// owning task.
type Tag struct {
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
	// (parented to the req_in, e.g. KindPipeline) spanning the same interval,
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
