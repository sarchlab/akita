package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
)

// A PortOwner is an element that can communicate with others through ports.
type PortOwner interface {
	AddPort(name string, port Port)
	GetPortByName(name string) Port
	Ports() []Port
}

// A Component is an element that owns ports and can be notified of port
// activity.
type Component interface {
	naming.Named
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}
