package akita

// A SimulationEndHandler is a handler that is called after the simulation ends.
type SimulationEndHandler interface {
	Handle(now VTimeInSec)
}

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	Hookable

	// Schedule registers an event to happen in the future
	Schedule(e Event)

	// Run will process all the events until the simulation finishes
	Run() error

	// CurrentTime will return the time at which the engine is at.
	CurrentTime() VTimeInSec

	// RegisterSimulationEndHandler registers a handler that perform some
	// actions after the simulation is finished.
	RegisterSimulationEndHandler(now VTimeInSec)

	// Finished invokes all the registered SimulationEndHandler
	Finished()
}
