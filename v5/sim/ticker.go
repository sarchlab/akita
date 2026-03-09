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
func MakeTickEvent(handler Handler, time VTimeInSec) TickEvent {
	evt := TickEvent{}
	evt.ID = GetIDGenerator().Generate()
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
	_ Port,
) {
	c.TickLater()
}

// NotifyRecv triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyRecv(
	_ Port,
) {
	c.TickLater()
}

// Handle triggers the tick function of the TickingComponent
func (c *TickingComponent) Handle(e Event) error {
	madeProgress := c.ticker.Tick()
	if madeProgress {
		c.TickLater()
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
