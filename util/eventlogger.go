package util

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// EventLogger is an hook that prints the event information
type EventLogger struct {
	LogHookBase
}

// NewEventLogger returns a new LogEventHook which will write in to the logger
func NewEventLogger(logger *log.Logger) *EventLogger {
	h := new(EventLogger)
	h.Logger = logger
	return h
}

// Type always return the type of Event
func (h *EventLogger) Type() reflect.Type {
	return reflect.TypeOf((core.Event)(nil))
}

// Pos of a PrintEventHook suggests that it should be called before the
// event handling.
func (h *EventLogger) Pos() core.HookPos {
	return core.BeforeEvent
}

// Func writes the event information into the logger
func (h *EventLogger) Func(
	item interface{},
	domain core.Hookable,
	info interface{},
) {
	evt := item.(core.Event)
	h.Logger.Printf("%.10f, %s -> %s", evt.Time(), reflect.TypeOf(evt),
		reflect.TypeOf(evt.Handler()))
}
