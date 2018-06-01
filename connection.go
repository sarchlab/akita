package core

// A Sender can send requests to their destinations
type Sender interface {
	Send(req Req) *Error
}

// A Receiver can receive requests
type Receiver interface {
	Recv(req Req) *Error
}

// An Error of the conn package is an error from the connection system.
//
// When a component checks if a Sender or a Reveicer CanSend or CanRecv a
// request, if the answer is no, an ConnError will be returned together.
//
// Recoverable determines if a later retry can solve the problem
// EarliestRetry give suggestions on earliest time to retry
type Error struct {
	Message       string
	Recoverable   bool
	EarliestRetry VTimeInSec
}

func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new ConnError
func NewError(name string, recoverable bool, earliestRetry VTimeInSec) *Error {
	return &Error{name, recoverable, earliestRetry}
}

// A Connectable is an object that an Connection can connect with.
type Connectable interface {
	AddPort(name string)
	Connect(portName string, conn Connection)
	GetConnection(portName string) Connection
	Disconnect(portName string)

	Receiver
}

// A Connection is responsible for delievering the requests to its
// destination.
type Connection interface {
	Sender
	Handler

	Attach(s Connectable)
	Detach(s Connectable)
}

// PlugIn links a Connection with a Component port
func PlugIn(comp Component, port string, connection Connection) {
	comp.Connect(port, connection)
	connection.Attach(comp)
}

// DeferredSend is an event that is designed for sending some
// information later.
//
// In discrete event simulation field, it is very common for sending
// some information right after an event. The request cannot be sent
// right at the event time due to the contention of resources.
// Therefore, DeferredSend provides a convenient data structure for this
// common pattern
type DeferredSend struct {
	*EventBase

	Req Req
}

// NewDeferredSend creates a new DefferedSend event
func NewDeferredSend(req Req) *DeferredSend {
	ds := new(DeferredSend)
	ds.EventBase = NewEventBase(req.SendTime(), req.Src())
	ds.Req = req
	return ds
}

// Time of the DeferredSend is always equal to the send time of the request
func (e *DeferredSend) Time() VTimeInSec {
	return e.Req.SendTime()
}

// SetTime sets the request sene time
func (e *DeferredSend) SetTime(t VTimeInSec) {
	e.Req.SetSendTime(t)
}
