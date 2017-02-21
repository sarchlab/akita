package event

// VTimeInSec defines the time in the simulated space in the unit of second
type VTimeInSec float64

// An Event is something going to happen in the future.
//
// Different from the concept of event of traditional discrete event simulation,
// event in Yaotsu can only be scheduled within by one event handler to
// itself. An event that is schedule by a handler can only modify that paticular
// handler or send requests over a Connection.
type Event interface {
	// Return the time that the event should happen
	Time() VTimeInSec

	// Returns the handler that can should handle the event
	Handler() Handler

	// When the handler finished processing the event, return on this channel
	FinishChan() chan bool
}

// BasicEvent provides the basic fields and getters for other events
type BasicEvent struct {
	time       VTimeInSec
	handler    Handler
	finishChan chan bool
}

// NewBasicEvent creates a new BasicEvent
func NewBasicEvent() *BasicEvent {
	return &BasicEvent{0, nil, make(chan bool)}
}

// SetTime sets when then event will happen
func (e *BasicEvent) SetTime(t VTimeInSec) {
	e.time = t
}

// Time returne the time that the event is going to happen
func (e *BasicEvent) Time() VTimeInSec {
	return e.time
}

// SetHandler sets which component will handle the event
//
// Yaotsu requires all the components can only schedule event for themselves.
// Therefore, the handler in this function must be the component who schedule
// the event. The only exception is process of kicking starting of the
// simulation, where the kick starter can schedule to all components.
func (e *BasicEvent) SetHandler(h Handler) {
	e.handler = h
}

// Handler returns the handler to handle the event
func (e *BasicEvent) Handler() Handler {
	return e.handler
}

// FinishChan return the channel to be used to signal the completion of the
// the event
func (e *BasicEvent) FinishChan() chan bool {
	return e.finishChan
}

// A Handler defines a domain for the events.
//
// One event is always constraint to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event) error
}
