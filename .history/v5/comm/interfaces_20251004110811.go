package comm

import (
	hooking "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"
	"github.com/sarchlab/akita/v4/v5/timing"
)

// Named describes any entity that exposes a human-readable identifier.
type Named interface {
	Name() string
}

// TimeTeller surfaces the current simulation time in seconds. Wiring ports use
// it to guard against retrieving a message within the same cycle it was sent.
type TimeTeller interface {
	CurrentTime() timing.VTimeInSec
}

// SendError marks failures when enqueuing or delivering messages.
type SendError struct{}

// NewSendError constructs a new SendError instance.
func NewSendError() *SendError {
	return &SendError{}
}

// Connection delivers messages between ports. Concrete implementations may
// impose different buffering semantics but must honour the notification
// contract exposed by Port.
type Connection interface {
	Named
	hooking.Hookable

	PlugIn(port Port)
	Unplug(port Port)
	NotifyAvailable(port Port)
	NotifySend()
}

// Component owns ports and reacts to inbound traffic and freed buffers.
type Component interface {
	Named

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}

// Port represents the messaging boundary between a component and a connection.
//
// Implementations may expose distinct buffering strategies yet must respect the
// contract defined below so they can operate interchangeably.
type Port interface {
	Named
	hooking.Hookable

	AsRemote() RemotePort

	SetConnection(conn Connection)
	Component() Component

	// Connection-facing APIs.
	Deliver(msg Msg) *SendError
	NotifyAvailable()
	RetrieveOutgoing() Msg
	PeekOutgoing() Msg

	// Component-facing APIs.
	CanSend() bool
	Send(msg Msg) *SendError
	RetrieveIncoming() Msg
	PeekIncoming() Msg
}
