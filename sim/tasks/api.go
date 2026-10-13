package tasks

import (
	"github.com/sarchlab/akita/v5/sim/internal/taskcore"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Owner supplies the task service and clock of one simulation. ComponentBase
// implements Owner without requiring any task hooks.
type Owner = taskcore.Owner

// Start begins work and returns an independent task ID to retain in component
// State. Disabled tracking returns zero without consulting the clock or ID
// generator. Simulation behavior must not depend on numeric ID values.
func Start(o Owner, info StartInfo) uint64 {
	return taskcore.Start(o, info)
}

// End completes a task, including one started by another owner in the same
// simulation. Zero and unknown IDs are ignored (for example after restore).
func End(o Owner, info EndInfo) {
	taskcore.End(o, info)
}

// AddTag forwards a categorical label for an open task.
func AddTag(o Owner, tag Tag) {
	taskcore.AddTag(o, tag)
}

// AddMilestone records when a blocking condition is resolved for an open task.
func AddMilestone(o Owner, m Milestone) {
	taskcore.AddMilestone(o, m)
}

// StartReqOut begins a sender task. Call before port.Send so child buffer tasks
// can resolve their parent, and retain the returned ID until FinalizeReq.
func StartReqOut(o Owner, msg messaging.Msg, parent uint64) uint64 {
	return taskcore.StartReqOut(o, msg, parent)
}

// StartReqIn begins the receiver's handling task; retain its ID until CompleteReq.
func StartReqIn(o Owner, msg messaging.Msg) uint64 {
	return taskcore.StartReqIn(o, msg)
}

// CompleteReq ends receiver work using its stored task ID.
func CompleteReq(o Owner, id uint64) {
	taskcore.CompleteReq(o, id)
}

// FinalizeReq ends sender work using its stored task ID.
func FinalizeReq(o Owner, id uint64) {
	taskcore.FinalizeReq(o, id)
}

// EndOnReset ends work discarded by a component reset. Unknown IDs are harmless.
func EndOnReset(o Owner, id uint64) {
	taskcore.EndOnReset(o, id)
}
