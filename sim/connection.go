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


