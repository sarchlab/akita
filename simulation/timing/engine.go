package timing

import (
	"context"

	"github.com/sarchlab/akita/v5/simulation/hooking"
)

// TimeTeller can be used to get the current time.
type TimeTeller interface {
	CurrentTime() VTimeInPicoSec
}

// EventScheduler can be used to schedule future events.
type EventScheduler interface {
	TimeTeller

	Schedule(e Event)
}

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	hooking.Hookable
	EventScheduler

	// RegisterHandler registers handler under name. An event names the
	// handler that receives it with HandlerID, so a handler must be
	// registered before an event is scheduled for it. A component's Build
	// registers the component; a handler written without a component model
	// registers itself.
	RegisterHandler(name string, handler Handler)

	// Run will process all the events until the simulation finishes.
	Run() error

	// RequestPause is safe in handlers; wait for its acknowledgment externally.
	RequestPause() PauseRequest

	// Pause waits for the current event/batch and hooks to finish.
	Pause() error

	// Continue resumes dispatch; repeated calls are harmless.
	Continue() error

	// IsPaused reports acknowledged state, including pauses requested by handlers.
	IsPaused() bool

	// State reports running, pausing, paused, or resuming without blocking.
	State() EngineState

	// Inspect copies/serializes state at a boundary without changing pause state.
	Inspect(context.Context, func() error) error
}
