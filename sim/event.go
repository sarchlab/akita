package sim

// VTimeInSec defines the time in the simulated space in the unit of second
type VTimeInSec float64

// An Event is something going to happen in the future.
type Event interface {
	// Return the time that the event should happen
	Time() VTimeInSec

	// Returns the handler that can should handle the event
	Handler() Handler

	// IsSecondary tells if the event is a secondary event. Secondary event are
	// handled after all same-time primary events are handled.
	IsSecondary() bool
}

// EventBase provides the basic fields and getters for other events
type EventBase struct {
	ID        string
	time      VTimeInSec
	handler   Handler
	secondary bool
}

// NewEventBase creates a new EventBase
func NewEventBase(t VTimeInSec, handler Handler) *EventBase {
	e := new(EventBase)
	e.ID = GetIDGenerator().Generate()
	e.time = t
	e.handler = handler
	e.secondary = false
	return e
}

// Time return the time that the event is going to happen
func (e EventBase) Time() VTimeInSec {
	return e.time
}

// SetHandler sets which handler that handles the event.
//
// Akita requires all the components can only schedule event for themselves.
// Therefore, the handler in this function must be the component who schedule
// the event. The only exception is process of kicking starting of the
// simulation, where the kick starter can schedule to all components.
func (e EventBase) SetHandler(h Handler) {
	e.handler = h
}

// Handler returns the handler to handle the event.
func (e EventBase) Handler() Handler {
	return e.handler
}

// IsSecondary returns true if the event is a secondary event.
func (e EventBase) IsSecondary() bool {
	return e.secondary
}

// A Handler defines a domain for the events.
//
// One event is always constraint to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event) error
}
