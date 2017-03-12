package main

import "gitlab.com/yaotsu/core"

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*core.BasicEvent
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent() *PingSendEvent {
	return &PingSendEvent{core.NewBasicEvent(), nil, nil}
}
