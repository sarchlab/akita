package twowaybuffered

import (
	"reflect"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// bufferEntry keeps lifecycle state beside the message and checkpoints both.
type bufferEntry struct {
	Message           messaging.Msg         `json:"message"`
	TaskID            uint64                `json:"task_id"`
	LastMilestoneTime timing.VTimeInPicoSec `json:"last_milestone_time"`
}

var _ tasks.Owner = (*Port)(nil)

// BindTaskTracker supplies the port's simulation task service once during setup.
// Simulation.RegisterPort calls it automatically. Standalone ports may bind
// explicitly before buffering messages. A nil tracker disables task tracking.
func (p *Port) BindTaskTracker(tracker *tasks.Tracker) {
	if p.taskTrackerBound {
		panic("twowaybuffered: task tracker already bound")
	}
	p.taskTracker = tracker
	p.taskTrackerBound = true
}

// AddIncomingMilestone records a resolved wait on the current incoming head.
func (p *Port) AddIncomingMilestone(reason messaging.WaitReason) {
	if p.taskTracker == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.headMilestone(&p.incomingBuf, p.waitReason(reason, true))
}

// AddOutgoingMilestone records a resolved wait on the current outgoing head.
func (p *Port) AddOutgoingMilestone(reason messaging.WaitReason) {
	if p.taskTracker == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.headMilestone(&p.outgoingBuf, p.waitReason(reason, false))
}

func (p *Port) startBufferTask(msg messaging.Msg, incoming bool) bufferEntry {
	entry := bufferEntry{Message: msg}
	tracker := p.taskTracker
	if tracker == nil {
		return entry
	}
	kind, direction := tasks.KindOutgoingBuffer, ".outgoing"
	if incoming {
		kind, direction = tasks.KindIncomingBuffer, ".incoming"
	}
	what := "metadata"
	if typ := reflect.TypeOf(msg.Payload); typ != nil {
		what = typ.Name()
		if what == "" {
			what = typ.String()
		}
	}
	entry.TaskID = tracker.StartTask(p, tasks.StartInfo{MsgID: msg.ID, Kind: kind,
		What: what, Location: p.name + direction, Detail: msg})
	entry.LastMilestoneTime = tracker.CurrentTime()
	return entry
}

func (p *Port) waitReason(reason messaging.WaitReason, incoming bool) messaging.WaitReason {
	if reason.Kind != "" {
		return reason
	}
	if incoming {
		name := ""
		if owner := p.owner; owner != nil {
			name = owner.Name()
		}
		return tasks.Reason(tasks.MilestoneKindConsumer, name)
	}
	name := ""
	if p.conn != nil {
		name = p.conn.Name()
	}
	return tasks.Reason(tasks.MilestoneKindNetworkBusy, name)
}

func (p *Port) headMilestone(buf *queueing.Buffer[bufferEntry], reason messaging.WaitReason) {
	entry, ok := buf.Peek()
	if !ok {
		return
	}
	p.bufferMilestone(&entry, reason)
	buf.UpdateFront(entry)
}

func (p *Port) bufferMilestone(entry *bufferEntry, reason messaging.WaitReason) {
	if entry.TaskID == 0 {
		return
	}
	tracker := p.taskTracker
	if tracker == nil {
		return
	}
	now := tracker.CurrentTime()
	if now <= entry.LastMilestoneTime {
		return
	}
	tracker.AddMilestone(tasks.Milestone{TaskID: entry.TaskID,
		Kind: tasks.MilestoneKind(reason.Kind), What: reason.What})
	entry.LastMilestoneTime = now
}

func (p *Port) endBufferTask(entry *bufferEntry, incoming bool) {
	if entry.TaskID == 0 {
		return
	}
	tracker := p.taskTracker
	if tracker == nil {
		return
	}
	p.bufferMilestone(entry, p.waitReason(messaging.WaitReason{}, incoming))
	tracker.EndTask(entry.TaskID)
}
