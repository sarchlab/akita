package messaging

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/naming"
)

// A Connection is responsible for delivering messages to its destination.
type Connection interface {
	naming.Named
	hooking.Hookable

	PlugIn(port Port)
	NotifyAvailable(port Port)

	NotifySend()
}
