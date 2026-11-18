package pagetablemmu

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// A milestone accounts for the interval since the previous one on this
// request. Coalesced requests retain their own admission times and boundaries.
type requestTrace struct {
	Task          uint64
	LastMilestone timing.VTimeInPicoSec
}

func startRequest(c *Comp, msg messaging.Msg) requestTrace {
	t := c.TaskTracker()
	if t == nil {
		return requestTrace{}
	}
	return requestTrace{Task: t.StartReqIn(c, msg), LastMilestone: t.CurrentTime()}
}

func (r *requestTrace) mark(t *tasks.Tracker, kind tasks.MilestoneKind, what string) {
	if t == nil || r.Task == 0 {
		return
	}
	now := t.CurrentTime()
	if now > r.LastMilestone {
		t.AddMilestone(tasks.Milestone{TaskID: r.Task, Kind: kind, What: what})
		r.LastMilestone = now
	}
}

func (w *walk) markWaiters(c *Comp, kind tasks.MilestoneKind, what string) {
	if t := c.TaskTracker(); t != nil {
		for i := range w.Waiters {
			w.Waiters[i].mark(t, kind, what)
		}
	}
}

func (w *walk) startLookupTask(c *Comp, stage string) {
	if t := c.TaskTracker(); t != nil {
		name := c.Name() + stage
		w.markWaiters(c, tasks.MilestoneKindHardwareResource, name)
		w.LookupTask = t.StartTask(c, tasks.StartInfo{
			ParentID: w.Waiters[0].Task, Kind: tasks.KindPipeline, What: name,
		})
	}
}

func (w *walk) finishLookupTask(c *Comp, stage string) {
	if t := c.TaskTracker(); t != nil {
		name := c.Name() + stage
		w.Waiters[0].mark(t, tasks.MilestoneKindWork, name)
		for i := 1; i < len(w.Waiters); i++ {
			w.Waiters[i].mark(t, tasks.MilestoneKindDependency, name)
		}
		t.EndTask(w.LookupTask)
		w.LookupTask = 0
	}
}

func (w *walk) markDecision(c *Comp, device uint64, kind tasks.MilestoneKind) {
	if t := c.TaskTracker(); t != nil {
		for i := range w.Waiters {
			if w.Waiters[i].Device == device {
				w.Waiters[i].mark(t, kind, c.Ports.PageFault.Name())
			}
		}
	}
}
