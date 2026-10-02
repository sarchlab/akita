package messaging

// A PortOwner owns ports and is notified of their activity. It is usually a
// component (modeling.Component). Which ports it has, and how it reaches them,
// is up to the owner; a port knows its owner through Port.Owner.
type PortOwner interface {
	// NotifyRecv is called when the port receives a message into an empty
	// incoming buffer.
	NotifyRecv(port Port)

	// NotifyPortFree is called when the port can send again.
	NotifyPortFree(port Port)
}
