package wakeup

import (
	"encoding/json"
	"maps"
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
// delivered, so at most one undelivered wakeup is pending for a given time.
// Another wakeup at the same time may be asked for after delivery.
type scheduler struct {
	lock       sync.Mutex
	handlerID  string
	simulation modeling.Simulation

	guard wakeGuard
}

// A promoted wakeup has a second event to keep its time live. After either
// copy delivers the work, its partner is ignored by ID because a handler can
// immediately request a fresh wakeup at the very same time.
type wakeGuard struct {
	Pending    map[timing.VTimeInPicoSec]wake `json:"pending"`
	Superseded map[uint64]bool                `json:"superseded,omitempty"`
}

type wake struct {
	ID         uint64 `json:"id"`
	Background bool   `json:"background,omitempty"`
	Promotion  uint64 `json:"promotion,omitempty"`
}

// Older checkpoints record foreground wakeups by time without an event ID.
func (g *wakeGuard) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '[' {
		var times []timing.VTimeInPicoSec
		if err := json.Unmarshal(data, &times); err != nil {
			return err
		}
		g.Pending = make(map[timing.VTimeInPicoSec]wake, len(times))
		for _, at := range times {
			g.Pending[at] = wake{}
		}
		return nil
	}
	type fields wakeGuard
	return json.Unmarshal(data, (*fields)(g))
}

func newScheduler(handlerID string, sim modeling.Simulation) *scheduler {
	return &scheduler{handlerID: handlerID, simulation: sim,
		guard: wakeGuard{Pending: make(map[timing.VTimeInPicoSec]wake)}}
}

// WakeAt schedules a foreground wakeup at t.
func (s *scheduler) WakeAt(t timing.VTimeInPicoSec) { s.schedule(t, false) }

// WakeAtBackground schedules a wakeup at t without keeping the engine running.
func (s *scheduler) WakeAtBackground(t timing.VTimeInPicoSec) { s.schedule(t, true) }

func (s *scheduler) schedule(t timing.VTimeInPicoSec, background bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	pending, found := s.guard.Pending[t]
	if found && (background || !pending.Background || pending.Promotion != 0) {
		return
	}
	id := s.simulation.NewID()
	if found {
		pending.Promotion = id
	} else {
		pending = wake{ID: id, Background: background}
	}
	s.guard.Pending[t] = pending
	event := Event{timing.MakeEventBase(id, t, s.handlerID)}
	event.Background = background
	s.simulation.Engine().Schedule(event)
}

func (s *scheduler) WakeNow() { s.WakeAt(s.simulation.Engine().CurrentTime()) }

func (s *scheduler) deliver(event Event) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.guard.Superseded[event.ID] {
		delete(s.guard.Superseded, event.ID)
		return false
	}
	pending, found := s.guard.Pending[event.At]
	if !found || (pending.ID != 0 && pending.ID != event.ID && (pending.Promotion == 0 || pending.Promotion != event.ID)) {
		return true
	}
	if pending.Promotion != 0 {
		other := pending.Promotion
		if other == event.ID {
			other = pending.ID
		}
		if s.guard.Superseded == nil {
			s.guard.Superseded = make(map[uint64]bool)
		}
		s.guard.Superseded[other] = true
	}
	delete(s.guard.Pending, event.At)
	return true
}

func (s *scheduler) Snapshot() wakeGuard {
	s.lock.Lock()
	defer s.lock.Unlock()
	return wakeGuard{Pending: maps.Clone(s.guard.Pending), Superseded: maps.Clone(s.guard.Superseded)}
}

func (s *scheduler) Restore(guard wakeGuard) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if guard.Pending == nil {
		guard.Pending = make(map[timing.VTimeInPicoSec]wake)
	}
	s.guard = guard
}
