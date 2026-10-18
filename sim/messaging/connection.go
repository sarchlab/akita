package messaging

import (
	"github.com/sarchlab/akita/v5/sim/naming"
)

// A Connection is responsible for delivering messages to its destination.
type Connection interface {
	naming.Named

	// BindPort attaches an owned, named port during setup. Implementations
	// reject ports incompatible with their transport before changing bindings.
	BindPort(port Port)
	NotifyAvailable(port Port)

	NotifySend(port Port)
}
