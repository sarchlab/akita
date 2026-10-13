package tasks

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Owner supplies the task service and clock of one simulation. ComponentBase
// implements Owner without requiring any task hooks.
type Owner interface {
	naming.Named
	timing.TimeTeller
	NewID() uint64
	TaskTracker() *Tracker
}

// Start begins work and returns an independent task ID to retain in component
// State. Disabled tracking returns zero without consulting the clock or ID
// generator. Simulation behavior must not depend on numeric ID values.
func Start(o Owner, info StartInfo) uint64 {
	t := o.TaskTracker()
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.start(o, info)
}

// End completes a task, including one started by another owner in the same
// simulation. Zero and unknown IDs are ignored (for example after restore).
func End(o Owner, info EndInfo) {
	t := o.TaskTracker()
	if t == nil || info.ID == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.end(o, info.ID)
}

// AddTag forwards a categorical label for an open task.
func AddTag(o Owner, tag Tag) {
	t := o.TaskTracker()
	if t == nil || tag.TaskID == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.open[tag.TaskID]
	if e == nil {
		return
	}
	if tag.ID == 0 {
		tag.ID = o.NewID()
	}
	tag.Time = o.CurrentTime()
	t.notify(e.owner, func(observer Observer) { observer.TagAdded(tag) })
}

// AddMilestone records when a blocking condition is resolved for an open task.
func AddMilestone(o Owner, m Milestone) {
	t := o.TaskTracker()
	if t == nil || m.TaskID == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.milestone(o, m)
}

// StartReqOut begins a sender task. Call before port.Send so child buffer tasks
// can resolve their parent, and retain the returned ID until FinalizeReq.
func StartReqOut(o Owner, msg messaging.Msg, parent uint64) uint64 {
	if o.TaskTracker() == nil {
		return 0
	}
	return Start(o, StartInfo{MsgID: msg.ID, ParentID: parent, Kind: KindReqOut, What: msgTypeName(msg), Detail: msg})
}

// StartReqIn begins the receiver's handling task; retain its ID until CompleteReq.
func StartReqIn(o Owner, msg messaging.Msg) uint64 {
	if o.TaskTracker() == nil {
		return 0
	}
	return Start(o, StartInfo{MsgID: msg.ID, Kind: KindReqIn, What: msgTypeName(msg), Detail: msg})
}

// CompleteReq ends receiver work using its stored task ID.
func CompleteReq(o Owner, id uint64) { End(o, EndInfo{ID: id}) }

// FinalizeReq ends sender work using its stored task ID.
func FinalizeReq(o Owner, id uint64) { End(o, EndInfo{ID: id}) }

// EndOnReset ends work discarded by a component reset. Unknown IDs are harmless.
func EndOnReset(o Owner, id uint64) { End(o, EndInfo{ID: id}) }

func msgTypeName(msg messaging.Msg) string {
	t := reflect.TypeOf(msg.Payload)
	if t == nil {
		return "metadata"
	}
	if name := t.Name(); name != "" {
		return name
	}
	return t.String()
}

func singleKindLocation(name, kind, what string) string {
	switch kind {
	case KindReqIn, KindReqOut:
		return name + "." + kind
	case KindPipeline:
		return what
	default:
		return name
	}
}
