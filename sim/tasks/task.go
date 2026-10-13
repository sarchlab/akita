// Package tasks owns simulation-scoped task lifecycles and typed notifications.
package tasks

import "github.com/sarchlab/akita/v5/sim/internal/taskcore"

// StartInfo describes work to begin. Start allocates the ID and stamps the time.
type StartInfo = taskcore.StartInfo

// EndInfo identifies work to complete. End stamps the time from the owner.
type EndInfo = taskcore.EndInfo

// Task is the complete basic record delivered at start and end. EndTime is
// populated at end. Tags and milestones are separate events, not replayed history.
// Detail is borrowed read-only data; observers must not mutate it.
type Task = taskcore.Task

// Shared task kinds keep trace consumers independent of component packages.
const (
	KindReqIn          = taskcore.KindReqIn
	KindReqOut         = taskcore.KindReqOut
	KindIncomingBuffer = taskcore.KindIncomingBuffer
	KindOutgoingBuffer = taskcore.KindOutgoingBuffer
	KindPipeline       = taskcore.KindPipeline
	KindMsgTransport   = taskcore.KindMsgTransport
	KindFlitTransport  = taskcore.KindFlitTransport
)

// A Tag is a categorical label attached to a task while it is processed,
// for example "read-hit" or "write-miss". Tags inherit their location from the
// owning task.
type Tag = taskcore.Tag

// MilestoneKind categorizes the blocking condition a milestone resolves.
type MilestoneKind = taskcore.MilestoneKind

const (
	MilestoneKindHardwareResource = taskcore.MilestoneKindHardwareResource
	MilestoneKindNetworkTransfer  = taskcore.MilestoneKindNetworkTransfer
	MilestoneKindNetworkBusy      = taskcore.MilestoneKindNetworkBusy
	MilestoneKindQueue            = taskcore.MilestoneKindQueue
	MilestoneKindData             = taskcore.MilestoneKindData
	MilestoneKindDependency       = taskcore.MilestoneKindDependency
	MilestoneKindOther            = taskcore.MilestoneKindOther
	MilestoneKindTranslation      = taskcore.MilestoneKindTranslation
	// MilestoneKindSubTask marks a wait on a child subtask. Like
	// MilestoneKindWork it asserts internal activity, so it requires a
	// corresponding child subtask to exist (see "Coverage principles" in the
	// package README).
	MilestoneKindSubTask = taskcore.MilestoneKindSubTask
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
	MilestoneKindWork = taskcore.MilestoneKindWork
)

// Milestone represents a point in time where a task's blocking status is
// resolved. Its location is inherited from the owning task.
type Milestone = taskcore.Milestone
