package core

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	Hookable

	// Schedule registers an event to happen in the future
	Schedule(e Event)

	// Run will process all the events until the simulation finishes
	Run() error

	// CurrentTime will return the time at which the engine is at.
	CurrentTime() VTimeInSec
}
