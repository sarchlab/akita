package wakeup

import (
	"sync"

	"github.com/sarchlab/akita/v5/timing"
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
// remembers the earliest pending wakeup, so asking for a wakeup at or after it
// schedules nothing.
type scheduler struct {
	lock       sync.Mutex
	handlerID  string
	simulation timing.Simulation

	at        timing.VTimeInPicoSec
	scheduled bool
}

// newScheduler creates a scheduler for the wakeup events of the handler with
// the given ID.
func newScheduler(handlerID string, sim timing.Simulation) *scheduler {
	return &scheduler{handlerID: handlerID, simulation: sim}
}

// WakeAt schedules a wakeup at time t, unless one is already pending at or
// before t.
func (s *scheduler) WakeAt(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.scheduled && s.at <= t {
		return
	}

	s.at = t
	s.scheduled = true

	s.simulation.Engine().Schedule(Event{
		EventBase: timing.MakeEventBase(s.simulation.NewID(), t, s.handlerID),
	})
}

// WakeNow schedules a wakeup at the current time.
func (s *scheduler) WakeNow() {
	s.WakeAt(s.simulation.Engine().CurrentTime())
}

// Woke clears the guard once the pending wakeup is delivered. The component
// calls it when it handles an Event at time t.
func (s *scheduler) Woke(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.scheduled && s.at <= t {
		s.scheduled = false
	}
}

// Snapshot returns the scheduler's dedup guard: whether a wakeup is pending
// and at what time. A component checkpoint saves it.
func (s *scheduler) Snapshot() (timing.VTimeInPicoSec, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.at, s.scheduled
}

// Restore sets the scheduler's dedup guard from a checkpoint. The engine
// restores the matching wakeup event separately. Only checkpoint loading
// should call it.
func (s *scheduler) Restore(at timing.VTimeInPicoSec, scheduled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.at = at
	s.scheduled = scheduled
}
