package akita

// SendError marks a failure send or receive
type SendError struct {
}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Connection is responsible for delivering the requests to its destination.
type Connection interface {
	Send(req Req) *SendError

	PlugIn(port *Port)
	Unplug(port *Port)
	NotifyAvailable(now VTimeInSec, port *Port)
}
