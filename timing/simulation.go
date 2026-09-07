package timing

// Simulation is the runtime context shared by elements in one simulation.
// ID allocation belongs to the simulation; its engine schedules events. This
// interface lets lower-level packages use the simulation without importing
// the concrete simulation package.
type Simulation interface {
	GetEngine() Engine
	NewID() uint64
	// GetIDGenerator returns the same counter for this simulation's lifetime.
	// Its identity distinguishes namespaces in shared tracing registries.
	GetIDGenerator() *IDGenerator
}

// SimulationElement belongs to a simulation. Elements use their simulation
// to allocate IDs, so the scope of an allocation is explicit at the call site.
type SimulationElement interface {
	Simulation() Simulation
}
