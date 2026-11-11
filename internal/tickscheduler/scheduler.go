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
// dispatches each edge at most once, including promotions to foreground.
type Scheduler struct {
	lock       sync.Mutex
	handlerID  string
	freq       timing.Freq
	simulation modeling.Simulation
	engine     timing.EventScheduler
	secondary  bool

	guard guardState
}

type guardState struct {
	Foreground  []timing.VTimeInPicoSec `json:"foreground"`
	Background  []timing.VTimeInPicoSec `json:"background,omitempty"`
	LastHandled timing.VTimeInPicoSec   `json:"last_handled,omitempty"`
	HasHandled  bool                    `json:"has_handled,omitempty"`
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

	if len(t.guard.Foreground) > 0 && t.guard.Foreground[0] <= t.freq.NextTick(now) {
		return
	}

	t.tickAt(now, t.freq.ThisTick(now), false)
}

// TickLater makes sure a tick is pending at the cycle after the current time.
func (t *Scheduler) TickLater() {
	t.lock.Lock()
	defer t.lock.Unlock()

	now := t.engine.CurrentTime()
	t.tickAt(now, t.freq.NextTick(now), false)
}

// TickAt makes sure a tick is pending at the first clock edge at or after time
// at; a time in the past asks for the current cycle. It schedules nothing
// only when a tick is already pending on that edge.
//
// The tick carries no data. A component keeps the work due at a later time
// in its State, calls TickAt with the due time when it adds the work, and
// handles the work that is due on each tick.
func (t *Scheduler) TickAt(at timing.VTimeInPicoSec) {
	t.lock.Lock()
	defer t.lock.Unlock()

	now := t.engine.CurrentTime()
	t.tickAt(now, t.freq.ThisTick(max(at, now)), false)
}

// TickAtBackground requests a tick which does not keep the engine running.
func (t *Scheduler) TickAtBackground(at timing.VTimeInPicoSec) {
	t.lock.Lock()
	defer t.lock.Unlock()
	now := t.engine.CurrentTime()
	t.tickAt(now, t.freq.ThisTick(max(at, now)), true)
}

func (t *Scheduler) tickAt(now, edge timing.VTimeInPicoSec, background bool) {
	t.prune(now)
	if t.guard.HasHandled && t.guard.LastHandled == edge {
		return
	}
	pending := &t.guard.Foreground
	index, found := slices.BinarySearch(*pending, edge)
	if found {
		return
	}
	if background {
		pending = &t.guard.Background
		index, found = slices.BinarySearch(*pending, edge)
		if found {
			return
		}
	}
	*pending = slices.Insert(*pending, index, edge)
	tick := MakeEvent(t.simulation.NewID(), edge, t.handlerID)
	tick.Secondary = t.secondary
	tick.Background = background
	t.engine.Schedule(tick)
}

// Deliver coalesces background and foreground events scheduled for the same
// edge. Either can reach Handle first in the parallel engine.
func (t *Scheduler) Deliver(edge timing.VTimeInPicoSec) bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.guard.HasHandled && t.guard.LastHandled == edge {
		_, foreground := slices.BinarySearch(t.guard.Foreground, edge)
		_, background := slices.BinarySearch(t.guard.Background, edge)
		return !(foreground && background)
	}
	t.guard.LastHandled = edge
	t.guard.HasHandled = true
	return true
}

func (t *Scheduler) prune(now timing.VTimeInPicoSec) {
	t.guard.Foreground = prune(t.guard.Foreground, now)
	t.guard.Background = prune(t.guard.Background, now)
}

func prune(pending []timing.VTimeInPicoSec, now timing.VTimeInPicoSec) []timing.VTimeInPicoSec {
	i, _ := slices.BinarySearch(pending, now)
	return slices.Delete(pending, 0, i)
}

// Snapshot saves pending foreground and background edges and the last delivery.
func (t *Scheduler) Snapshot() guardState {
	t.lock.Lock()
	defer t.lock.Unlock()
	state := t.guard
	state.Foreground = slices.Clone(state.Foreground)
	state.Background = slices.Clone(state.Background)
	return state
}

// Restore reinstates the scheduler metadata for the restored engine events.
func (t *Scheduler) Restore(state guardState) {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.guard = state
}
