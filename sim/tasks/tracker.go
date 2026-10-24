package tasks

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

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
	clock     timing.TimeTeller
	newID     func() uint64
	owns      func(naming.Named) bool
	mu        sync.Mutex
	observers []Observer       // one per subscription, in subscription order
	unscoped  []int            // indexes into observers of subscriptions without scopes
	byScope   map[string][]int // scope name -> indexes into observers, ascending
	open      map[uint64]*entry
	parents   requestParents
}

// NewTracker creates a service using the simulation's clock, ID allocator, and
// scope membership check. owns must identify objects in that simulation's
// inventory, not merely match names. Applications normally use
// sim.Builder.WithTaskTracking instead of constructing this service themselves.
func NewTracker(clock timing.TimeTeller, newID func() uint64, owns func(naming.Named) bool) *Tracker {
	if clock == nil || newID == nil || owns == nil {
		panic("tasks: simulation services are required")
	}
	return &Tracker{clock: clock, newID: newID, owns: owns,
		open: make(map[uint64]*entry), byScope: make(map[string][]int)}
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
		if named == nil || !t.owns(named) {
			panic("tasks: subscription scope belongs to another simulation or is not a task owner")
		}
		scope[named.Name()] = true
	}
	index := len(t.observers)
	t.observers = append(t.observers, o)
	if len(scope) == 0 {
		t.unscoped = append(t.unscoped, index)
		return
	}
	for name := range scope {
		t.byScope[name] = append(t.byScope[name], index)
	}
}

func (t *Tracker) start(o Owner, info StartInfo) uint64 {
	if info.Kind == "" || info.What == "" || o.Name() == "" {
		panic("tasks: owner name, kind, and what must be set")
	}
	if info.Location == "" {
		info.Location = singleKindLocation(o.Name(), info.Kind, info.What)
	}
	task := Task{ID: t.newID(), MsgID: info.MsgID, ParentID: info.ParentID,
		Kind: info.Kind, What: info.What, Location: info.Location, Detail: info.Detail, StartTime: t.clock.CurrentTime()}
	if task.ID == 0 {
		panic("tasks: simulation allocated zero ID")
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

func (t *Tracker) end(id uint64) {
	e := t.open[id]
	if e == nil {
		return
	}
	task := e.task
	task.EndTime = t.clock.CurrentTime()
	delete(t.open, id)
	t.parents.end(id)
	t.notify(e, func(observer Observer) { observer.TaskEnded(task) })
}

func (t *Tracker) milestone(m Milestone) {
	e := t.open[m.TaskID]
	if e == nil {
		return
	}
	if m.ID == 0 {
		m.ID = t.newID()
	}
	m.Time = t.clock.CurrentTime()
	t.notify(e, func(observer Observer) { observer.MilestoneAdded(m) })
}

// notify selects by the starting owner even when another component ends work.
// It visits only the matching subscriptions, so its cost does not grow with the
// number of subscriptions scoped to other owners. Matching subscriptions are
// notified once each, in subscription order, even when one names both a port and
// its component.
func (t *Tracker) notify(e *entry, callback func(Observer)) {
	lists := [3][]int{t.unscoped, t.byScope[e.owner]}
	if e.component != "" && e.component != e.owner {
		lists[2] = t.byScope[e.component]
	}
	if len(lists[0]) == 0 && len(lists[2]) == 0 {
		for _, index := range lists[1] {
			callback(t.observers[index])
		}
		return
	}
	for {
		next := -1
		for _, list := range lists {
			if len(list) > 0 && (next < 0 || list[0] < next) {
				next = list[0]
			}
		}
		if next < 0 {
			return
		}
		for i := range lists {
			if len(lists[i]) > 0 && lists[i][0] == next {
				lists[i] = lists[i][1:]
			}
		}
		callback(t.observers[next])
	}
}

// CurrentTime returns the simulation clock used for all task notifications.
// Port buffer bookkeeping uses the same clock for its checkpointed wait boundary.
func (t *Tracker) CurrentTime() timing.VTimeInPicoSec { return t.clock.CurrentTime() }
