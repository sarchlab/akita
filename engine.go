package core

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {

	// Schedule registers an event to be happen in the future
	Schedule(e Event)

	// Run will process all the events until the simulation finishes
	Run() error

	// Pause will temporarily stops the engine from triggering more events.
	Pause()
}
