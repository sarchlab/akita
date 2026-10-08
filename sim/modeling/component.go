package modeling

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// A Component is an element of a simulation that owns ports and handles the
// events addressed to it. The Component of each model (modeling/ticking,
// modeling/wakeup, and modeling/event) implements it. Build registers the instance and its event handler.
// BindPort assigns its ports, and simulation.Initialize creates runtime state.
type Component interface {
	naming.Named
	hooking.Hookable
	timing.Handler
	messaging.PortOwner
}

// None is the Resources type of a component that references no shared
// objects, and the State type of one that keeps no state.
type None struct{}
