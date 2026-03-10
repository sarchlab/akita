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
	Named
	Hookable

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(port Port)

	// V5: Add port information to NotifySend. Knowing the port is helpful
	// for the wire implementation to notify the right destination.
	NotifySend()
}

// HookPosConnStartSend marks a connection accept to send a message.
var HookPosConnStartSend = &HookPos{Name: "Conn Start Send"}

// HookPosConnStartTrans marks a connection start to transmit a message.
var HookPosConnStartTrans = &HookPos{Name: "Conn Start Trans"}

// HookPosConnDoneTrans marks a connection complete transmitting a message.
var HookPosConnDoneTrans = &HookPos{Name: "Conn Done Trans"}

// HookPosConnDeliver marks a connection delivered a message.
var HookPosConnDeliver = &HookPos{Name: "Conn Deliver"}
