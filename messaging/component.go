package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// A Component is an element that owns ports, is notified of their activity,
// and handles the events addressed to it. Which ports it has, and how it
// reaches them, is up to the component; a port knows its component through
// Port.Component.
type Component interface {
	naming.Named
	hooking.Hookable
	timing.Handler

	NotifyRecv(port Port)
	NotifyPortFree(port Port)
}
