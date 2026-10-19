package tasks

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

// StartTask begins work and returns an independent task ID to retain in component
// State. Disabled tracking returns zero without consulting the clock or ID
// generator. Simulation behavior must not depend on numeric ID values.
func (t *Tracker) StartTask(o Owner, info StartInfo) uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.start(o, info)
}

// EndTask completes a task, including one started by another owner in the same
// simulation. Zero and unknown IDs are ignored (for example after restore).
func (t *Tracker) EndTask(id uint64) {
	if t == nil || id == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.end(id)
}

// AddTag forwards a categorical label for an open task.
func (t *Tracker) AddTag(tag Tag) {
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
		tag.ID = t.newID()
	}
	tag.Time = t.clock.CurrentTime()
	t.notify(e, func(observer Observer) { observer.TagAdded(tag) })
}

// AddMilestone records when a blocking condition is resolved for an open task.
func (t *Tracker) AddMilestone(m Milestone) {
	if t == nil || m.TaskID == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.milestone(m)
}

// StartReqOut begins a sender task. Call before port.Send so child buffer tasks
// can resolve their parent, and retain the returned ID until EndReqOut.
func (t *Tracker) StartReqOut(o Owner, msg messaging.Msg, parent uint64) uint64 {
	if t == nil {
		return 0
	}
	return t.StartTask(o, StartInfo{MsgID: msg.ID, ParentID: parent,
		Kind: KindReqOut, What: msgTypeName(msg), Detail: msg})
}

// StartReqIn begins the receiver's handling task; retain its ID until EndReqIn.
func (t *Tracker) StartReqIn(o Owner, msg messaging.Msg) uint64 {
	if t == nil {
		return 0
	}
	return t.StartTask(o, StartInfo{MsgID: msg.ID, Kind: KindReqIn, What: msgTypeName(msg), Detail: msg})
}

// EndReqIn ends receiver work using the ID returned by StartReqIn.
// Zero and unknown IDs are ignored, including repeated ends during reset.
func (t *Tracker) EndReqIn(id uint64) { t.EndTask(id) }

// EndReqOut ends sender work using the ID returned by StartReqOut.
// Zero and unknown IDs are ignored, including repeated ends during reset.
func (t *Tracker) EndReqOut(id uint64) { t.EndTask(id) }

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
