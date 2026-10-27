package wakeup

import (
	"slices"
	"sync"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Event wakes a component that has no clock. It carries no data: the
// component's middlewares look at its State and ports to find the work that
// is ready.
type Event struct {
	timing.EventBase
}

// init registers Event so the engine's event queue can be checkpointed while
// a wakeup is pending. Wakeups are scheduled by value.
func init() {
	timing.RegisterEvent(Event{})
}

// scheduler schedules the wakeup events of a handler. Its dedup guard
// remembers the time of every wakeup it scheduled that has not been
// delivered, so asking for a wakeup at or after the earliest one schedules
// nothing, and at most one undelivered wakeup is pending for a given time.
// Another wakeup at the same time may be asked for after delivery.
type scheduler struct {
	lock       sync.Mutex
	handlerID  string
	simulation modeling.Simulation

	// pending holds the times of the undelivered wakeups in ascending order.
	pending []timing.VTimeInPicoSec
}

// newScheduler creates a scheduler for the wakeup events of the handler with
// the given ID.
func newScheduler(handlerID string, sim modeling.Simulation) *scheduler {
	return &scheduler{handlerID: handlerID, simulation: sim}
}

// WakeAt schedules a wakeup at time t, unless one is already pending at or
// before t.
func (s *scheduler) WakeAt(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.pending) > 0 && s.pending[0] <= t {
		return
	}

	s.pending = slices.Insert(s.pending, 0, t)

	s.simulation.Engine().Schedule(Event{
		EventBase: timing.MakeEventBase(s.simulation.NewID(), t, s.handlerID),
	})
}

// WakeNow schedules a wakeup at the current time.
func (s *scheduler) WakeNow() {
	s.WakeAt(s.simulation.Engine().CurrentTime())
}

// Woke forgets the wakeups at or before t once the one at t is delivered, so
// a wakeup can be asked for again at the same time. The component calls it
// when it handles an Event at time t.
func (s *scheduler) Woke(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	i, found := slices.BinarySearch(s.pending, t)
	if found {
		i++
	}

	s.pending = slices.Delete(s.pending, 0, i)
}

// Snapshot returns the scheduler's dedup guard: the times of the undelivered
// wakeups. A component checkpoint saves it.
func (s *scheduler) Snapshot() []timing.VTimeInPicoSec {
	s.lock.Lock()
	defer s.lock.Unlock()

	return slices.Clone(s.pending)
}

// Restore sets the scheduler's dedup guard from a checkpoint. The engine
// restores the matching wakeup events separately. Only checkpoint loading
// should call it.
func (s *scheduler) Restore(pending []timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.pending = slices.Sorted(slices.Values(pending))
}
