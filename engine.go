package core

import (
	"log"
	"reflect"
)

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

// LogEventHook is an hook that prints the event information
type LogEventHook struct {
	LogHookBase
}

// NewLogEventHook returns a new LogEventHook which will write in to the logger
func NewLogEventHook(logger *log.Logger) *LogEventHook {
	h := new(LogEventHook)
	h.Logger = logger
	return h
}

// Type always return the type of a Event
func (h *LogEventHook) Type() reflect.Type {
	return reflect.TypeOf((Event)(nil))
}

// Pos of a PrintEventHook suggests that it should be called before the
// event handling.
func (h *LogEventHook) Pos() HookPos {
	return BeforeEvent
}

// Func writes the event information into the logger
func (h *LogEventHook) Func(
	item interface{},
	domain Hookable,
	info interface{},
) {
	evt := item.(Event)
	h.Logger.Printf("%.10f, %s -> %s", evt.Time(), reflect.TypeOf(evt),
		reflect.TypeOf(evt.Handler()))
}
