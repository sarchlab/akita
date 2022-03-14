package sim

// TimeTeller can be used to get the current time.
type TimeTeller interface {
	CurrentTime() VTimeInSec
}

// EventScheduler can be used to schedule future events.
type EventScheduler interface {
	Schedule(e Event)
}

// A SimulationEndHandler is a handler that is called after the simulation ends.
type SimulationEndHandler interface {
	Handle(now VTimeInSec)
}

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	Hookable
	TimeTeller
	EventScheduler

	// Run will process all the events until the simulation finishes
	Run() error

	// Pause will pause the simulation until continue is called.
	Pause()

	// Continue will continue the paused simulation
	Continue()

	// RegisterSimulationEndHandler registers a handler that perform some
	// actions after the simulation is finished.
	RegisterSimulationEndHandler(handler SimulationEndHandler)

	// Finished invokes all the registered SimulationEndHandler
	Finished()
}
