package tickscheduler

import (
	"sync"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// Event is the event a ticking component receives every cycle while it
// makes progress. It carries no data.
type Event struct {
	timing.EventBase
}

// MakeEvent creates a new Event.
func MakeEvent(id uint64, time timing.VTimeInPicoSec, handlerID string) Event {
	return Event{EventBase: timing.MakeEventBase(id, time, handlerID)}
}

// init registers Event so the engine's event queue can be checkpointed
// while a tick is pending. Ticks are scheduled by value.
func init() {
	timing.RegisterEvent(Event{})
}

// Scheduler schedules the tick events of a handler. Its dedup guard remembers
// the pending tick, so at most one tick per cycle is scheduled.
type Scheduler struct {
	lock       sync.Mutex
	handlerID  string
	freq       timing.Freq
	simulation timing.Simulation
	engine     timing.EventScheduler
	secondary  bool

	nextTickTime     timing.VTimeInPicoSec
	hasScheduledTick bool
}

// NewScheduler creates a scheduler for the tick events of the handler with
// the given ID.
func NewScheduler(
	handlerID string,
	sim timing.Simulation,
	freq timing.Freq,
) *Scheduler {
	ticker := new(Scheduler)

	ticker.handlerID = handlerID
	ticker.simulation = sim
	ticker.engine = sim.Engine()
	ticker.freq = freq
	ticker.hasScheduledTick = false

	return ticker
}

// NewSecondaryScheduler creates a scheduler that always schedules secondary
// tick events, which run after the primary events of the same time.
func NewSecondaryScheduler(
	handlerID string,
	sim timing.Simulation,
	freq timing.Freq,
) *Scheduler {
	ticker := new(Scheduler)

	ticker.handlerID = handlerID
	ticker.simulation = sim
	ticker.engine = sim.Engine()
	ticker.freq = freq
	ticker.secondary = true
	ticker.hasScheduledTick = false

	return ticker
}

// TickNow schedule a Tick event at the current time.
func (t *Scheduler) TickNow() {
	t.lock.Lock()
	time := t.engine.CurrentTime()

	if t.hasScheduledTick && t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = t.freq.ThisTick(time)
	t.hasScheduledTick = true
	tick := MakeEvent(t.simulation.NewID(), t.nextTickTime, t.handlerID)

	if t.secondary {
		tick.Secondary = true
	}

	t.engine.Schedule(tick)
	t.lock.Unlock()
}

// TickLater will schedule a tick event at the cycle after the now time.
func (t *Scheduler) TickLater() {
	t.lock.Lock()
	time := t.freq.NextTick(t.engine.CurrentTime())

	if t.hasScheduledTick && t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	t.hasScheduledTick = true
	tick := MakeEvent(t.simulation.NewID(), t.nextTickTime, t.handlerID)

	if t.secondary {
		tick.Secondary = true
	}

	t.engine.Schedule(tick)
	t.lock.Unlock()
}

// Snapshot returns the scheduler's dedup guard: whether a tick is pending and
// at what time. A component checkpoint saves it so the guard can be restored
// directly, with no post-load reconciliation against the event queue.
func (t *Scheduler) Snapshot() (nextTickTime timing.VTimeInPicoSec, scheduled bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.nextTickTime, t.hasScheduledTick
}

// Restore sets the scheduler's dedup guard from a checkpoint. The engine
// restores the matching tick event separately, so the two stay consistent.
// Only checkpoint loading should call it.
func (t *Scheduler) Restore(nextTickTime timing.VTimeInPicoSec, scheduled bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.nextTickTime = nextTickTime
	t.hasScheduledTick = scheduled
}
