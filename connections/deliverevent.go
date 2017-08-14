package connections

import "gitlab.com/yaotsu/core"

// DeliverEvent is the event that moves the request from the connection
// to the destination
type DeliverEvent struct {
	*core.EventBase

	Req core.Req
}

// NewDeliverEvent creates a new DeliverEvent
func NewDeliverEvent(t core.VTimeInSec, handler core.Handler, req core.Req) *DeliverEvent {
	e := new(DeliverEvent)
	e.EventBase = core.NewEventBase(t, handler)
	e.Req = req
	return e
}
