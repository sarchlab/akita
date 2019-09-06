package akita

// SendError marks a failure send or receive
type SendError struct{}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Connection is responsible for delivering the msguests to its destination.
type Connection interface {
	Hookable

	Send(msg Msg) *SendError

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(now VTimeInSec, port Port)
}

// HookPosConnStartSend marks a connection accept to send a msguest
var HookPosConnStartSend = &HookPos{Name: "Conn Start Send"}

// HookPosConnStartTrans marks a connection start to transmit a msguest
var HookPosConnStartTrans = &HookPos{Name: "Conn Start Trans"}

// HookPosConnDoneTrans marks a connection complete transmitting a msguest
var HookPosConnDoneTrans = &HookPos{Name: "Conn Done Trans"}

// HookPosConnDeliver marks a connection delivered a msguest
var HookPosConnDeliver = &HookPos{Name: "Conn Deliver"}
