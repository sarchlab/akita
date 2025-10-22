package simv5

import "github.com/sarchlab/akita/v5/instrumentation/hooking"

// TimeTeller provides the current simulation time.
type TimeTeller interface {
	CurrentTime() VTimeInSec
}

// EventScheduler provides the ability to schedule events in the simulation.
type EventScheduler interface {
	TimeTeller

	// Schedule adds an event to the simulation queue.
	// The event will be delivered to the specified handler at the specified time.
	Schedule(item ScheduleItem)
}

// Engine manages the discrete event simulation.
// It maintains an event queue, processes events in time order,
// and provides hooks for instrumentation.
type Engine interface {
	hooking.Hookable
	EventScheduler

	// Run processes all events until the simulation completes.
	Run() error

	// Pause pauses the simulation. The simulation can be resumed with Continue.
	Pause()

	// Continue resumes a paused simulation.
	Continue()

	// Finished returns true if there are no more events to process.
	Finished() bool
}
