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

	Send(msg Msg) *SendError

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(now VTimeInSec, port Port)
}

// HookPosConnStartSend marks a connection accept to send a request
var HookPosConnStartSend = &HookPos{Name: "Conn Start Send"}

// HookPosConnStartTrans marks a connection start to transmit a request
var HookPosConnStartTrans = &HookPos{Name: "Conn Start Trans"}

// HookPosConnDoneTrans marks a connection complete transmitting a request
var HookPosConnDoneTrans = &HookPos{Name: "Conn Done Trans"}

// HookPosConnDeliver marks a connection delivered a request
var HookPosConnDeliver = &HookPos{Name: "Conn Deliver"}
