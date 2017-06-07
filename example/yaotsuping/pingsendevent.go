package main

import "gitlab.com/yaotsu/core"

// A PingSendEvent is an event scheduled for sending a ping
type PingSendEvent struct {
	*core.EventBase
	From *PingComponent
	To   *PingComponent
}

// NewPingSendEvent creates a new PingSendEvent
func NewPingSendEvent(
	time core.VTimeInSec,
	handler core.Handler,
) *PingSendEvent {
	return &PingSendEvent{core.NewEventBase(time, handler), nil, nil}
}
