package tasks

import (
	"sync"
)

type subscription struct {
	observer Observer
	owners   map[string]bool
}
type entry struct {
	task  Task
	owner string
}

// Tracker owns the open tasks and message correlation for one simulation.
// It is opt-in and does not retain event history. Simulation wiring checkpoints
// its open records and correlation, excluding Detail and observer state.
// Its initial implementation serializes notifications under one lock; observers
// must still support the concurrency contract so storage can be sharded later.
// Construct a Tracker with NewTracker; its zero value is not ready for use.
type Tracker struct {
	mu            sync.Mutex
	frozen        bool
	subscriptions []subscription
	open          map[uint64]*entry
	parents       requestParents
	buffers       map[bufferKey]uint64
	ports         map[string]bool
}

// NewTracker creates a service for one simulation. Framework assembly passes
// the same tracker to every owner and registers their ports before running.
// Applications normally enable it with sim.Builder.WithTaskTracking.
func NewTracker() *Tracker {
	return &Tracker{open: make(map[uint64]*entry),
		buffers: make(map[bufferKey]uint64), ports: make(map[string]bool)}
}

// Subscribe registers an observer before the first engine run. With no owners
// it observes the entire simulation. Owner scopes include their port tasks.
func (t *Tracker) Subscribe(o Observer, owners ...Owner) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.frozen {
		panic("tasks: subscriptions are frozen after the first run")
	}
	if o == nil {
		panic("tasks: nil observer")
	}
	scope := make(map[string]bool, len(owners))
	for _, owner := range owners {
		if owner == nil || owner.TaskTracker() != t {
			panic("tasks: subscription owner belongs to another simulation")
		}
		scope[owner.Name()] = true
	}
	t.subscriptions = append(t.subscriptions, subscription{observer: o, owners: scope})
}

// FreezeSubscriptions closes observer and port registration before the first
// engine run. Framework run guards call it; a nil tracker is a no-op.
func (t *Tracker) FreezeSubscriptions() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.frozen = true
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
	t.open[task.ID] = &entry{task: task, owner: o.Name()}
	t.notify(o.Name(), func(observer Observer) { observer.TaskStarted(task) })
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
	t.notify(e.owner, func(observer Observer) { observer.TaskEnded(task) })
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
	t.notify(e.owner, func(observer Observer) { observer.MilestoneAdded(m) })
}

// notify selects by the starting owner even when another component ends work.
func (t *Tracker) notify(owner string, callback func(Observer)) {
	for _, sub := range t.subscriptions {
		if len(sub.owners) == 0 || sub.owners[owner] {
			callback(sub.observer)
		}
	}
}
