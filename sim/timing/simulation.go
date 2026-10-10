package timing

import "github.com/sarchlab/akita/v5/sim/naming"

// Simulation supplies runtime services and registers the elements in one simulation.
// ID allocation belongs to the simulation; its engine schedules events. This
// interface lets lower-level packages use the simulation without importing
// the concrete simulation package.
type Simulation interface {
	Engine() Engine
	NewID() uint64

	Initialize() error
	RequireSetup()
	RequireNameAvailable(name string)

	// Registration adds elements to the simulation's inventory for checkpointing,
	// lookup, tracing, and monitoring. Standalone contexts retain the inventory for initialization.
	RegisterComponent(c naming.Named)
	RegisterConnection(c naming.Named)
	RegisterResource(c naming.Named)
	RegisterPort(p naming.Named)
}
