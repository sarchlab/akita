package main

import "gitlab.com/yaotsu/core"

// A PingReturnEvent is an event scheduled for returning the ping request
type PingReturnEvent struct {
	*core.EventBase
	Req *PingReq
}

// NewPingReturnEvent creates a new PingReturnEvent
func NewPingReturnEvent(
	t core.VTimeInSec,
	handler core.Handler,
) *PingReturnEvent {
	return &PingReturnEvent{core.NewBasicEvent(t, handler), nil}
}
