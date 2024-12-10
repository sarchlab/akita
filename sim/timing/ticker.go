package timing

import (
	"sync"

	"github.com/sarchlab/akita/v4/sim/id"
)

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	EventBase
}

// MakeTickEvent creates a new TickEvent
func MakeTickEvent(handler Handler, time VTimeInSec) TickEvent {
	evt := TickEvent{}
	evt.ID = id.Generate()
	evt.handler = handler
	evt.time = time
	evt.secondary = false

	return evt
}

// A Ticker is an object that updates states with ticks.
type Ticker interface {
	Tick() bool
}

// TickScheduler can help schedule tick events.
type TickScheduler struct {
	lock      sync.Mutex
	handler   Handler
	Freq      Freq
	Engine    Engine
	secondary bool

	nextTickTime VTimeInSec
}

// NewTickScheduler creates a scheduler for tick events.
func NewTickScheduler(
	handler Handler,
	engine Engine,
	freq Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handler = handler
	ticker.Engine = engine
	ticker.Freq = freq
	ticker.nextTickTime = -1 // This will make sure the first tick is scheduled

	return ticker
}

// NewSecondaryTickScheduler creates a scheduler that always schedule secondary
// tick events.
func NewSecondaryTickScheduler(
	handler Handler,
	engine Engine,
	freq Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handler = handler
	ticker.Engine = engine
	ticker.Freq = freq
	ticker.secondary = true
	ticker.nextTickTime = -1 // This will make sure the first tick is scheduled

	return ticker
}

// TickNow schedule a Tick event at the current time.
func (t *TickScheduler) TickNow() {
	t.lock.Lock()
	time := t.CurrentTime()

	if t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = t.Freq.ThisTick(time)
	tick := MakeTickEvent(t.handler, t.nextTickTime)

	if t.secondary {
		tick.secondary = true
	}

	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// TickLater will schedule a tick event at the cycle after the now time.
func (t *TickScheduler) TickLater() {
	t.lock.Lock()
	time := t.Freq.NextTick(t.CurrentTime())

	if t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	tick := MakeTickEvent(t.handler, t.nextTickTime)

	if t.secondary {
		tick.secondary = true
	}

	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

func (t *TickScheduler) CurrentTime() VTimeInSec {
	return t.Engine.CurrentTime()
}
