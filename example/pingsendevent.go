package example

import "gitlab.com/yaotsu/core/event"

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*event.BasicEvent
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent() *PingSendEvent {
	return &PingSendEvent{event.NewBasicEvent(), nil, nil}
}
