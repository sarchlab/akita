package core

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	// Engines are hookable for all the requests
	Hookable

	// Schedule registers an event to be happen in the future
	Schedule(e Event)

	// Run will process all the events until the simulation finishes
	Run() error

	// Pause will temporarily stops the engine from triggering more events.
	Pause()
}
