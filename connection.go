package akita

// SendError marks a failure send or receive
type SendError struct{}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Connection is responsible for delivering the requests to its destination.
type Connection interface {
	Hookable

	Send(req Req) *SendError

	PlugIn(port *Port)
	Unplug(port *Port)
	NotifyAvailable(now VTimeInSec, port *Port)
}

var ConnStartSendHookPos = struct{ name string }{"Conn Start Send"}
var ConnStartTransHookPos = struct{ name string }{"Conn Start Trans"}
var ConnDoneTransHookPos = struct{ name string }{"Conn Done Trans"}
var ConnDeliverHookPos = struct{ name string }{"Conn Deliver"}
