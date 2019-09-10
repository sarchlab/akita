package akita

import (
	"sync"

	"github.com/rs/xid"
)

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	EventBase
}

// NewTickEvent creates a newly created TickEvent
func NewTickEvent(t VTimeInSec, handler Handler) *TickEvent {
	evt := new(TickEvent)
	evt.EventBase = EventBase{}
	evt.EventBase.ID = xid.New().String()
	evt.EventBase.handler = handler
	evt.EventBase.time = t
	return evt
}

// Ticker is a tool that helps a component that executes in a tick-tick fashion
type Ticker struct {
	lock    sync.Mutex
	handler Handler
	Freq    Freq
	Engine  Engine

	nextTickTime VTimeInSec
}

// NewTicker creates a new ticker
func NewTicker(handler Handler, engine Engine, freq Freq) *Ticker {
	ticker := new(Ticker)

	ticker.handler = handler
	ticker.Engine = engine
	ticker.Freq = freq

	ticker.nextTickTime = -1
	return ticker
}

// TickLater will continue with ticking.
func (t *Ticker) TickLater(now VTimeInSec) {
	t.lock.Lock()

	time := t.Freq.NextTick(now)

	if t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	tick := TickEvent{}
	tick.ID = xid.New().String()
	tick.time = time
	tick.handler = t.handler
	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// A TickingComponent is a component that mainly updates its states in a
// cycle-based fashion.
//
// A TickingComponent represents a pattern that can be used to avoid busy
// ticking.
// When the component receives a request or receives a notification that
// a port is getting available, it starts to tick. At the beginning of the
// processing the TickEvent, it sets the NeedTick field to false. While
// the Component updates its internal states, it determines if any
// progress is made. If the Component makes any progress, the NeedTick
// field should be set to True. Otherwise, the field remains false.
// After updating the states, the Component schedules next tick event if
// the NeedTick field is true.
type TickingComponent struct {
	*ComponentBase
	*Ticker
	NeedTick bool
}

// NotifyPortFree triggers the TickingComponent to continue to tick.
func (c *TickingComponent) NotifyPortFree(now VTimeInSec, port Port) {
	c.Ticker.TickLater(now)
}

// NotifyRecv triggers the TickingComponent to continue to tick.
func (c *TickingComponent) NotifyRecv(now VTimeInSec, port Port) {
	c.Ticker.TickLater(now)
}

// NewTickingComponent creates a new ticking component
func NewTickingComponent(
	name string,
	engine Engine,
	freq Freq,
	handler Handler,
) *TickingComponent {
	tickingComponent := new(TickingComponent)
	tickingComponent.Ticker = NewTicker(handler, engine, freq)
	tickingComponent.ComponentBase = NewComponentBase(name)
	return tickingComponent
}
