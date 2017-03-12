package main

import "gitlab.com/yaotsu/core"

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*core.BasicEvent
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent() *PingReturnEvent {
	return &PingReturnEvent{core.NewBasicEvent(), nil}
}
