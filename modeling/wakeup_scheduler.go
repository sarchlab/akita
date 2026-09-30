package modeling

import (
	"sync"

	"github.com/sarchlab/akita/v5/timing"
)

// WakeupEvent wakes a component that has no clock (see modeling/wakeup). It
// carries no data: the component's middlewares look at its State and ports to
// find the work that is ready.
type WakeupEvent struct {
	timing.EventBase
}

// WakeupScheduler schedules a component's wakeup events. Its dedup guard
// remembers the earliest pending wakeup, so asking for a wakeup at or after it
// schedules nothing.
type WakeupScheduler struct {
	lock       sync.Mutex
	handlerID  string
	simulation timing.Simulation

	at        timing.VTimeInPicoSec
	scheduled bool
}

// NewWakeupScheduler creates a scheduler for the wakeup events of the handler
// with the given ID.
func NewWakeupScheduler(handlerID string, sim timing.Simulation) *WakeupScheduler {
	return &WakeupScheduler{handlerID: handlerID, simulation: sim}
}

// WakeAt schedules a wakeup at time t, unless one is already pending at or
// before t.
func (s *WakeupScheduler) WakeAt(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.scheduled && s.at <= t {
		return
	}

	s.at = t
	s.scheduled = true

	s.simulation.GetEngine().Schedule(WakeupEvent{
		EventBase: timing.MakeEventBase(s.simulation.NewID(), t, s.handlerID),
	})
}

// WakeNow schedules a wakeup at the current time.
func (s *WakeupScheduler) WakeNow() {
	s.WakeAt(s.simulation.GetEngine().CurrentTime())
}

// Woke clears the guard once the pending wakeup is delivered. The component
// calls it when it handles a WakeupEvent at time t.
func (s *WakeupScheduler) Woke(t timing.VTimeInPicoSec) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.scheduled && s.at <= t {
		s.scheduled = false
	}
}

func (s *WakeupScheduler) snapshot() (timing.VTimeInPicoSec, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.at, s.scheduled
}

func (s *WakeupScheduler) restore(at timing.VTimeInPicoSec, scheduled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.at = at
	s.scheduled = scheduled
}
