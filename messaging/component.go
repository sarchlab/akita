package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// A PortOwner is an element that can communicate with others through ports.
// Which ports it has is fixed when it is built; its port instances are
// supplied externally. Declaring ports is a construction-time concern and is
// not part of this interface.
type PortOwner interface {
	AssignPort(name string, port Port)
	GetPortByName(name string) Port
	AllPorts() []Port
}

// A Component is an element that owns ports and can be notified of port
// activity.
type Component interface {
	timing.SimulationElement
	naming.Named
	hooking.Hookable
	PortOwner

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}
