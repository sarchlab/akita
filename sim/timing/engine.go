package timing

import (
	"context"

	"github.com/sarchlab/akita/v5/sim/hooking"
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

	// SetRunGuard installs the simulation readiness check before every Run.
	// Set it during setup, before the engine can run.
	SetRunGuard(check func() error)

	// Run will process all the events until the simulation finishes.
	Run() error

	// RequestPause requests a pause without waiting for the current event or
	// parallel batch and its hooks to finish. Handlers and hooks may request a
	// pause, but PauseRequest.Wait must be called outside those callbacks.
	// Repeated requests are idempotent; a pause persists until Continue.
	RequestPause() PauseRequest

	// Continue waits for permission to resume dispatch, not for an event to run.
	// Call it outside handlers and hooks. Requests are applied in enqueue order;
	// repeated calls are idempotent.
	Continue() error

	// State reports dispatch permission and the next queued transition without
	// blocking: EngineRunning, EnginePausing, EnginePaused, or EngineResuming.
	// It is safe during Run, but does not report whether Run is active or failed.
	// Use EngineState.IsPaused to check whether dispatch is still paused.
	State() EngineState

	// Inspect copies/serializes state at a boundary without changing pause state.
	Inspect(context.Context, func() error) error
}
