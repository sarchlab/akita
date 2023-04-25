package sim

import (
	"sync"
)

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	EventBase
}

// MakeTickEvent creates a new TickEvent
func MakeTickEvent(t VTimeInSec, handler Handler) TickEvent {
	evt := TickEvent{}
	evt.ID = GetIDGenerator().Generate()
	evt.handler = handler
	evt.time = t
	evt.secondary = false
	return evt
}

// A Ticker is an object that updates states with ticks.
type Ticker interface {
	Tick(now VTimeInSec) bool
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

	ticker.nextTickTime = -1
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

	ticker.nextTickTime = -1
	return ticker
}

// TickNow schedule a Tick event at the current time.
func (t *TickScheduler) TickNow(now VTimeInSec) {
	t.lock.Lock()
	time := now

	if t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	tick := MakeTickEvent(time, t.handler)
	if t.secondary {
		tick.secondary = true
	}
	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// TickLater will schedule a tick event at the cycle after the now time.
func (t *TickScheduler) TickLater(now VTimeInSec) {
	t.lock.Lock()
	time := t.Freq.NextTick(now)

	if t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	tick := MakeTickEvent(time, t.handler)
	if t.secondary {
		tick.secondary = true
	}
	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// TickingComponent is a type of component that update states from cycle to
// cycle. A programmer would only need to program a tick function for a ticking
// component.
type TickingComponent struct {
	*ComponentBase
	*TickScheduler

	ticker Ticker
}

// NotifyPortFree triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyPortFree(
	now VTimeInSec,
	_ Port,
) {
	c.TickLater(now)
}

// NotifyRecv triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyRecv(
	now VTimeInSec,
	_ Port,
) {
	c.TickLater(now)
}

// Handle triggers the tick function of the TickingComponent
func (c *TickingComponent) Handle(e Event) error {
	now := e.Time()
	madeProgress := c.ticker.Tick(now)
	if madeProgress {
		c.TickLater(now)
	}
	return nil
}

// NewTickingComponent creates a new ticking component
func NewTickingComponent(
	name string,
	engine Engine,
	freq Freq,
	ticker Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = NewTickScheduler(tc, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker
	return tc
}

// NewSecondaryTickingComponent creates a new ticking component
func NewSecondaryTickingComponent(
	name string,
	engine Engine,
	freq Freq,
	ticker Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = NewSecondaryTickScheduler(tc, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker
	return tc
}
