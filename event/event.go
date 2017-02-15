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
	SetTime(time VTimeInSec)
	Time() VTimeInSec

	SetHandler(handler Handler)
	Handler() Handler

	// When the handler finished processing the event, return on this channel
	FinishChan() chan bool
}

// A Handler defines a domain for the events.
//
// One event is always constraint to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event)
}
