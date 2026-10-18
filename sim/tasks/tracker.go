package tasks

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
)

type subscription struct {
	observer Observer
	owners   map[string]bool
}
type entry struct {
	task      Task
	owner     string
	component string
}

// Tracker owns the open tasks and message correlation for one simulation.
// It is opt-in and does not retain event history. Simulation wiring checkpoints
// its open records and correlation, excluding Detail and observer state.
// Its initial implementation serializes notifications under one lock; observers
// must still support the concurrency contract so storage can be sharded later.
// Construct a Tracker with NewTracker; its zero value is not ready for use.
type Tracker struct {
	mu            sync.Mutex
	subscriptions []subscription
	open          map[uint64]*entry
	parents       requestParents
}

// NewTracker creates a service for one simulation. Framework assembly passes
// the same tracker to every owner and its ports.
// Applications normally enable it with sim.Builder.WithTaskTracking.
func NewTracker() *Tracker {
	return &Tracker{open: make(map[uint64]*entry)}
}

// Subscribe registers an observer for future events, including the complete end
// of work already in flight. No starts, tags, or milestones are replayed. With no
// scopes it observes the entire simulation. A component scope includes its ports;
// a port scope observes only that port. Every scope must use this tracker.
func (t *Tracker) Subscribe(o Observer, scopes ...naming.Named) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if o == nil {
		panic("tasks: nil observer")
	}
	scope := make(map[string]bool, len(scopes))
	for _, named := range scopes {
		owner, ok := named.(Owner)
		if !ok || owner.TaskTracker() != t {
			panic("tasks: subscription scope belongs to another simulation or is not a task owner")
		}
		scope[owner.Name()] = true
	}
	t.subscriptions = append(t.subscriptions, subscription{observer: o, owners: scope})
}

func (t *Tracker) start(o Owner, info StartInfo) uint64 {
	if info.Kind == "" || info.What == "" || o.Name() == "" {
		panic("tasks: owner name, kind, and what must be set")
	}
	if info.Location == "" {
		info.Location = singleKindLocation(o.Name(), info.Kind, info.What)
	}
	task := Task{ID: o.NewID(), MsgID: info.MsgID, ParentID: info.ParentID,
		Kind: info.Kind, What: info.What, Location: info.Location, Detail: info.Detail, StartTime: o.CurrentTime()}
	if task.ID == 0 {
		panic("tasks: owner allocated zero ID")
	}
	task = t.parents.resolveStart(task)
	e := &entry{task: task, owner: o.Name()}
	if port, ok := o.(messaging.Port); ok {
		if component := port.Owner(); component != nil {
			e.component = component.Name()
		}
	}
	t.open[task.ID] = e
	t.notify(e, func(observer Observer) { observer.TaskStarted(task) })
	return task.ID
}

func (t *Tracker) end(o Owner, id uint64) {
	e := t.open[id]
	if e == nil {
		return
	}
	task := e.task
	task.EndTime = o.CurrentTime()
	delete(t.open, id)
	t.parents.end(id)
	t.notify(e, func(observer Observer) { observer.TaskEnded(task) })
}

func (t *Tracker) milestone(o Owner, m Milestone) {
	e := t.open[m.TaskID]
	if e == nil {
		return
	}
	if m.ID == 0 {
		m.ID = o.NewID()
	}
	m.Time = o.CurrentTime()
	t.notify(e, func(observer Observer) { observer.MilestoneAdded(m) })
}

// notify selects by the starting owner even when another component ends work.
func (t *Tracker) notify(e *entry, callback func(Observer)) {
	for _, sub := range t.subscriptions {
		if len(sub.owners) == 0 || sub.owners[e.owner] || sub.owners[e.component] {
			callback(sub.observer)
		}
	}
}
