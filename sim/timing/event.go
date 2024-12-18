package timing

import (
	"reflect"

	"github.com/sarchlab/akita/v4/sim/hooking"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/serialization"
)

func init() {
	serialization.RegisterType(reflect.TypeOf((*Event)(nil)).Elem())
}

// VTimeInSec defines the time in the simulated space in the unit of second
type VTimeInSec = float64

// An Event is something going to happen in the future.
type Event interface {
	serialization.Serializable

	// Return the time that the event should happen
	Time() VTimeInSec

	// Returns the handler that can should handle the event
	Handler() Handler

	// IsSecondary tells if the event is a secondary event. Secondary event are
	// handled after all same-time primary events are handled.
	IsSecondary() bool
}

// HookPosBeforeEvent is a hook position that triggers before handling an event.
var HookPosBeforeEvent = &hooking.HookPos{Name: "BeforeEvent"}

// HookPosAfterEvent is a hook position that triggers after handling an event.
var HookPosAfterEvent = &hooking.HookPos{Name: "AfterEvent"}

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
	e.ID = id.Generate()
	e.time = t
	e.handler = handler
	e.secondary = false

	return e
}

// Time return the time that the event is going to happen
func (e EventBase) Time() VTimeInSec {
	return e.time
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
