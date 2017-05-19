package core

// DelieverEvent is the event that moves the request from the connection
// to the destination
type DelieverEvent struct {
	*EventBase

	Req Req
}

// NewDelieverEvent creates a new DelieverEvent
func NewDelieverEvent(t VTimeInSec, handler Handler, req Req) *DelieverEvent {
	e := new(DelieverEvent)
	e.EventBase = NewEventBase(t, handler)
	e.Req = req
	return e
}
