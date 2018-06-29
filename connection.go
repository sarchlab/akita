package core

// SendError marks a failure send or receive
type SendError struct {
}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Receiver can receive requests
type Receiver interface {
	Recv(req Req) *SendError
}

// A Connectable is an object that an Connection can connect with.
type Connectable interface {
	AddPort(name string)
	Connect(portName string, conn Connection)
	GetConnection(portName string) Connection
	Disconnect(portName string)

	Receiver
}

// A Connection is responsible for delivering the requests to its destination.
type Connection interface {
	Send(req Req) *SendError

	PlugIn(comp Connectable, port string)
	Unplug(comp Connectable, port string)
	NotifyAvailable(now VTimeInSec, comp Connectable)
}
