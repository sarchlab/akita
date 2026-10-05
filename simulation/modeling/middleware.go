package modeling

import "github.com/sarchlab/akita/v5/simulation/timing"

// Middleware is one piece of a component's behavior. Every component model
// uses it: the component passes each event it receives to its middlewares in
// the declaration order of its Middlewares struct. A tick is a
// ticking.TickEvent; a middleware ignores the events it does not handle. A
// component handles one event at a time, even under the parallel engine, so
// a middleware needs no locking to use the component's State.
type Middleware interface {
	// Handle processes an event. It returns true if it made progress:
	// changed the State, or sent or received a message.
	Handle(e timing.Event) bool
}
