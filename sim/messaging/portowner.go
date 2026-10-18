package messaging

import "github.com/sarchlab/akita/v5/sim/naming"

// A PortOwner owns ports and is notified of their activity. It is usually a
// component. Which ports it has, and how it reaches them,
// is up to the owner; a port knows its owner through Port.Owner.
type PortOwner interface {
	naming.Named

	// BindPort assigns the local slot, owner, and fully qualified port name.
	// Bind during setup, before connecting the port or initializing simulation.
	BindPort(slot string, port Port)

	// NotifyRecv is called when the port receives a message into an empty
	// incoming buffer.
	NotifyRecv(port Port)

	// NotifyPortFree is called when the port can send again.
	NotifyPortFree(port Port)
}
