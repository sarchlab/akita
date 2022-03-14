package sim

// SendError marks a failure send or receive
type SendError struct{}

// NewSendError creates a SendError
func NewSendError() *SendError {
	e := new(SendError)
	return e
}

// A Connection is responsible for delivering messages to its destination.
type Connection interface {
	Hookable

	CanSend(src Port) bool
	Send(msg Msg) *SendError

	// PlugIn connects a port to the connection. The connection should reserve
	// a buffer that can hold `sourceSideBufSize` messages.
	PlugIn(port Port, sourceSideBufSize int)
	Unplug(port Port)
	NotifyAvailable(now VTimeInSec, port Port)
}

// HookPosConnStartSend marks a connection accept to send a message.
var HookPosConnStartSend = &HookPos{Name: "Conn Start Send"}

// HookPosConnStartTrans marks a connection start to transmit a message.
var HookPosConnStartTrans = &HookPos{Name: "Conn Start Trans"}

// HookPosConnDoneTrans marks a connection complete transmitting a message.
var HookPosConnDoneTrans = &HookPos{Name: "Conn Done Trans"}

// HookPosConnDeliver marks a connection delivered a message.
var HookPosConnDeliver = &HookPos{Name: "Conn Deliver"}
