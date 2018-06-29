package core

// DeliverEvent is the event that moves the request from the connection
// to the destination
type DeliverEvent struct {
	*EventBase

	Req Req
}

// NewDeliverEvent creates a new DeliverEvent
func NewDeliverEvent(t VTimeInSec, handler Handler, req Req) *DeliverEvent {
	e := new(DeliverEvent)
	e.EventBase = NewEventBase(t, handler)
	e.Req = req
	return e
}
