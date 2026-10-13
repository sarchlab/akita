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

// TaskTracker returns the owning component's tracker, or nil for an untracked owner.
func (p *Port) TaskTracker() *tasks.Tracker {
	if owner, ok := p.owner.(tasks.Owner); ok {
		return owner.TaskTracker()
	}
	return nil
}

// CurrentTime uses the owning component's simulation clock.
func (p *Port) CurrentTime() timing.VTimeInPicoSec {
	if owner, ok := p.owner.(tasks.Owner); ok {
		return owner.CurrentTime()
	}
	return 0
}

// NewID uses the owning component's simulation ID allocator.
func (p *Port) NewID() uint64 {
	if owner, ok := p.owner.(tasks.Owner); ok {
		return owner.NewID()
	}
	return 0
}

// AddIncomingMilestone records a resolved wait on the current incoming head.
func (p *Port) AddIncomingMilestone(reason messaging.WaitReason) {
	if p.TaskTracker() == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.headMilestone(&p.incomingBuf, p.waitReason(reason, true))
}

// AddOutgoingMilestone records a resolved wait on the current outgoing head.
func (p *Port) AddOutgoingMilestone(reason messaging.WaitReason) {
	if p.TaskTracker() == nil {
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.headMilestone(&p.outgoingBuf, p.waitReason(reason, false))
}

func (p *Port) startBufferTask(msg messaging.Msg, incoming bool) bufferEntry {
	entry := bufferEntry{Message: msg}
	if p.TaskTracker() == nil {
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
	entry.TaskID = tasks.StartTask(p, tasks.StartInfo{MsgID: msg.ID, Kind: kind,
		What: what, Location: p.name + direction, Detail: msg})
	entry.LastMilestoneTime = p.CurrentTime()
	return entry
}

func (p *Port) waitReason(reason messaging.WaitReason, incoming bool) messaging.WaitReason {
	if reason.Kind != "" {
		return reason
	}
	if incoming {
		name := ""
		if owner, ok := p.owner.(tasks.Owner); ok {
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
	if entry.TaskID == 0 || p.TaskTracker() == nil {
		return
	}
	now := p.CurrentTime()
	if now <= entry.LastMilestoneTime {
		return
	}
	tasks.AddMilestone(p, tasks.Milestone{TaskID: entry.TaskID,
		Kind: tasks.MilestoneKind(reason.Kind), What: reason.What})
	entry.LastMilestoneTime = now
}

func (p *Port) endBufferTask(entry *bufferEntry, incoming bool) {
	if entry.TaskID == 0 || p.TaskTracker() == nil {
		return
	}
	p.bufferMilestone(entry, p.waitReason(messaging.WaitReason{}, incoming))
	tasks.EndTask(p, tasks.EndInfo{ID: entry.TaskID})
}
