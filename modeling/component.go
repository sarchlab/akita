package modeling

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// A Component is an element of a simulation that owns ports and handles the
// events addressed to it. The Component of each model (modeling/ticking,
// modeling/wakeup, and modeling/event) implements it. Build registers an
// instance as the owner of its ports, as the handler of its events, and as a
// component of the simulation.
type Component interface {
	naming.Named
	hooking.Hookable
	timing.Handler
	messaging.PortOwner
}

// None is the Resources type of a component that references no shared
// objects, and the State type of one that keeps no state.
type None struct{}
