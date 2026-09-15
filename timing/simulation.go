package timing

import "github.com/sarchlab/akita/v5/naming"

// Simulation supplies runtime services and registers the elements in one simulation.
// ID allocation belongs to the simulation; its engine schedules events. This
// interface lets lower-level packages use the simulation without importing
// the concrete simulation package.
type Simulation interface {
	GetEngine() Engine
	NewID() uint64
	// GetIDGenerator returns the same counter for this simulation's lifetime.
	// Its identity distinguishes namespaces in shared tracing registries.
	GetIDGenerator() *IDGenerator

	// Registration adds elements to the simulation's inventory for checkpointing,
	// lookup, tracing, and monitoring. Lightweight contexts may leave it empty.
	RegisterComponent(c naming.Named)
	RegisterConnection(c naming.Named)
	RegisterResource(c naming.Named)
	RegisterPort(p naming.Named)
}

// SimulationElement belongs to a simulation. Elements use their simulation
// to allocate IDs, so the scope of an allocation is explicit at the call site.
type SimulationElement interface {
	Simulation() Simulation
}
