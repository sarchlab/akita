package akita

import (
	"sync"
)

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
	lock    sync.Mutex
	handler Handler
	Freq    Freq
	Engine  Engine

	nextTickTime VTimeInSec
}

func NewTicker(handler Handler, engine Engine, freq Freq) *Ticker {
	ticker := new(Ticker)

	ticker.handler = handler
	ticker.Engine = engine
	ticker.Freq = freq

	ticker.nextTickTime = -1
	return ticker
}

func (t *Ticker) TickLater(now VTimeInSec) {
	t.lock.Lock()
	defer t.lock.Unlock()

	time := t.Freq.NextTick(now)

	if t.nextTickTime >= time {
		return
	}

	t.nextTickTime = time
	tick := NewTickEvent(time, t.handler)
	t.Engine.Schedule(tick)
}

// A Ticking Component is a component that mainly updates its states in a
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

func (c *TickingComponent) NotifyPortFree(now VTimeInSec, port *Port) {
	c.Ticker.TickLater(now)
}

func (c *TickingComponent) NotifyRecv(now VTimeInSec, port *Port) {
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
