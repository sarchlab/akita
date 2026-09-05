package timing

import (
	"context"

	"github.com/sarchlab/akita/v5/hooking"
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

// HandlerRegistrar allows registering named handlers for event dispatch.
type HandlerRegistrar interface {
	RegisterHandler(name string, handler Handler)
}

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	hooking.Hookable
	EventScheduler

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

	// Inspect copies/serializes state at a boundary without changing pause state.
	Inspect(context.Context, func() error) error
}
