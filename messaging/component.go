package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// A Component is an element that owns ports and is notified of their
// activity. Which ports it has, and how it reaches them, is up to the
// component; a port knows its component through Port.Component.
type Component interface {
	timing.SimulationElement
	naming.Named
	hooking.Hookable

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}
