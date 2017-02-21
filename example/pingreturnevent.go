package example

import "gitlab.com/yaotsu/core/event"

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*event.BasicEvent
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent() *PingReturnEvent {
	return &PingReturnEvent{event.NewBasicEvent(), nil}
}
