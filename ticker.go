package core

import "sync"

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	*EventBase
}

// NewTickEvent creates a newly created TickEvent
func NewTickEvent(t VTimeInSec, handler Handler) *TickEvent {
	evt := new(TickEvent)
	evt.EventBase = NewEventBase(t, handler)
	return evt
}

// Ticker is a tool that helps a component that executes in a tick-tick fashion
type Ticker struct {
	sync.Mutex
	handler Handler
	freq    Freq
	engine  Engine

	nextTickTime VTimeInSec
}

func NewTicker(handler Handler, engine Engine, freq Freq) *Ticker {
	ticker := new(Ticker)

	ticker.handler = handler
	ticker.engine = engine
	ticker.freq = freq

	ticker.nextTickTime = -1
	return ticker
}

func (t *Ticker) TickLater(now VTimeInSec) {
	t.Lock()
	defer t.Unlock()

	time := t.freq.NextTick(now)

	if t.nextTickTime >= time {
		return
	}

	t.nextTickTime = time
	tick := NewTickEvent(time, t.handler)
	t.engine.Schedule(tick)
}
