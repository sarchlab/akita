package tickscheduler

import (
	"slices"
	"sync"

	"github.com/sarchlab/akita/v5/sim/modeling"
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
// the clock edge of every tick it scheduled that may still be pending, so it
// never schedules two ticks on the same edge.
type Scheduler struct {
	lock       sync.Mutex
	handlerID  string
	freq       timing.Freq
	simulation modeling.Simulation
	engine     timing.EventScheduler
	secondary  bool

	// pending holds, in ascending order, the edges of the scheduled ticks at
	// or after the current time. A tick at the current time may already have
	// fired; it still counts for a request on the current edge, so a cycle
	// ticks at most once, but not for a request on a later edge, since a tick
	// that has fired asks for nothing more.
	pending []timing.VTimeInPicoSec
}

// NewScheduler creates a scheduler for the tick events of the handler with
// the given ID.
func NewScheduler(
	handlerID string,
	sim modeling.Simulation,
	freq timing.Freq,
) *Scheduler {
	ticker := new(Scheduler)

	ticker.handlerID = handlerID
	ticker.simulation = sim
	ticker.engine = sim.Engine()
	ticker.freq = freq

	return ticker
}

// NewSecondaryScheduler creates a scheduler that always schedules secondary
// tick events, which run after the primary events of the same time.
func NewSecondaryScheduler(
	handlerID string,
	sim modeling.Simulation,
	freq timing.Freq,
) *Scheduler {
	ticker := NewScheduler(handlerID, sim, freq)
	ticker.secondary = true

	return ticker
}

// TickNow schedules a tick at the current cycle, unless a tick is already
// pending at this cycle or the next.
func (t *Scheduler) TickNow() {
	t.lock.Lock()
	defer t.lock.Unlock()

	now := t.engine.CurrentTime()
	t.prune(now)

	if len(t.pending) > 0 && t.pending[0] <= t.freq.NextTick(now) {
		return
	}

	t.schedule(t.freq.ThisTick(now))
}

// TickLater schedules a tick at the cycle after the current time, unless one
// is already pending at that edge.
func (t *Scheduler) TickLater() {
	t.lock.Lock()
	defer t.lock.Unlock()

	now := t.engine.CurrentTime()
	t.tickAt(now, t.freq.NextTick(now))
}

// TickAt schedules a tick at the first clock edge at or after time at, unless
// a tick is already pending at or before that edge. A time in the past asks
// for the current cycle.
//
// The tick carries no data. A component keeps the work due at a later time
// in its State, calls TickAt when it adds the work, and, on each tick,
// handles the work that is due and asks again for the earliest work left.
func (t *Scheduler) TickAt(at timing.VTimeInPicoSec) {
	t.lock.Lock()
	defer t.lock.Unlock()

	now := t.engine.CurrentTime()
	t.tickAt(now, t.freq.ThisTick(max(at, now)))
}

// tickAt schedules a tick at edge, which is not before now, unless a tick is
// pending at edge or at an edge after now and before edge. Such an earlier
// tick will fire and the component asks again, while a tick at now may be the
// one being handled.
func (t *Scheduler) tickAt(now, edge timing.VTimeInPicoSec) {
	t.prune(now)

	for _, p := range t.pending {
		if p > edge {
			break
		}

		if p == edge || p > now {
			return
		}
	}

	t.schedule(edge)
}

// prune forgets the ticks before now, which have fired.
func (t *Scheduler) prune(now timing.VTimeInPicoSec) {
	i, _ := slices.BinarySearch(t.pending, now)
	t.pending = slices.Delete(t.pending, 0, i)
}

// schedule records a tick at edge, which is not pending, and schedules it.
func (t *Scheduler) schedule(edge timing.VTimeInPicoSec) {
	i, _ := slices.BinarySearch(t.pending, edge)
	t.pending = slices.Insert(t.pending, i, edge)

	tick := MakeEvent(t.simulation.NewID(), edge, t.handlerID)
	tick.Secondary = t.secondary

	t.engine.Schedule(tick)
}

// Snapshot returns the scheduler's dedup guard: the edges of the ticks that
// may still be pending. A component checkpoint saves it so the guard can be
// restored directly, with no post-load reconciliation against the event queue.
func (t *Scheduler) Snapshot() []timing.VTimeInPicoSec {
	t.lock.Lock()
	defer t.lock.Unlock()

	return slices.Clone(t.pending)
}

// Restore sets the scheduler's dedup guard from a checkpoint. The engine
// restores the matching tick events separately, so the two stay consistent.
// Only checkpoint loading should call it.
func (t *Scheduler) Restore(pending []timing.VTimeInPicoSec) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.pending = slices.Sorted(slices.Values(pending))
}
