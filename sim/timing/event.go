package timing

// VTimeInPicoSec defines the time in the simulated space in the unit of picosecond.
type VTimeInPicoSec uint64

// An Event is something going to happen in the future. Events keep Run alive
// by default. An event may also implement BackgroundEvent.
type Event interface {
	// Time returns the time that the event should happen.
	Time() VTimeInPicoSec

	// HandlerID returns the handler ID that can handle the event.
	HandlerID() string

	// IsSecondary tells if the event is a secondary event. Secondary events are
	// handled after all same-time primary events are handled.
	IsSecondary() bool
}

// BackgroundEvent optionally marks a scheduled event as background. Background
// work runs in time order while foreground work exists and otherwise stays queued.
// The scheduling call determines the kind; handlers do not inherit it.
type BackgroundEvent interface {
	IsBackground() bool
}

// EventBase provides the basic fields and getters for other events.
type EventBase struct {
	ID         uint64         `json:"id"`
	At         VTimeInPicoSec `json:"time"`
	Handler    string         `json:"handler_id"`
	Background bool           `json:"background,omitempty"`
	Secondary  bool           `json:"secondary"`
}

// MakeEventBase creates a new EventBase as a value.
func MakeEventBase(id uint64, t VTimeInPicoSec, handlerID string) EventBase {
	return EventBase{
		ID:        id,
		At:        t,
		Handler:   handlerID,
		Secondary: false,
	}
}

// Time returns the time that the event is going to happen.
func (e EventBase) Time() VTimeInPicoSec {
	return e.At
}

// HandlerID returns the handler ID to handle the event.
func (e EventBase) HandlerID() string {
	return e.Handler
}

// IsSecondary returns true if the event is a secondary event.
func (e EventBase) IsSecondary() bool {
	return e.Secondary
}

// IsBackground reports whether this event may remain queued at the end of Run.
func (e EventBase) IsBackground() bool { return e.Background }

// A Handler defines a domain for the events.
//
// One event is always constrained to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event)
}
