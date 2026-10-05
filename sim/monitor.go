package sim

import "github.com/sarchlab/akita/v5/sim/naming"

// Monitor observes a simulation without tying the runtime to a user interface.
// A factory binds each monitor to one simulation at construction. Build calls
// Start once, before application components or ports are registered, and
// forwards registrations as those objects are built. Terminate calls Stop
// before closing the tracer and data recorder owned by the simulation.
type Monitor interface {
	Start()
	RegisterComponent(c naming.Named)
	RegisterPort(p Port)
	Stop()
}
