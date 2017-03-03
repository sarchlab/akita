package conn

import "gitlab.com/yaotsu/core/event"

// A Sender can send requests to their destinations
type Sender interface {
	Send(req Request) *Error
}

// A Receiver can receive requests
type Receiver interface {
	Receive(req Request) *Error
}

// An Error of the conn package is an error from the connection system.
//
// When a component checks if a Sender or a Reveicer CanSend or CanRecv a
// request, if the answer is no, an ConnError will be returned together.
//
// Recoverable determines if a later retry can solve the problem
// EarliestRetry give suggestions on earliest time to retry
type Error struct {
	msg           string
	Recoverable   bool
	EarliestRetry event.VTimeInSec
}

func (e *Error) Error() string {
	return e.msg
}

// NewError creates a new ConnError
func NewError(name string, recoverable bool, earliestRetry event.VTimeInSec) *Error {
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

	Attach(s Connectable)
	Detach(s Connectable)
}

// PlugIn links a Connection with a Component port
func PlugIn(comp Component, port string, connection Connection) {
	comp.Connect(port, connection)
	connection.Attach(comp)
}
