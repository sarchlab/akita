package core

import (
	"log"
	"reflect"
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
	return reflect.TypeOf((Event)(nil))
}

// Pos of a PrintEventHook suggests that it should be called before the
// event handling.
func (h *EventLogger) Pos() HookPos {
	return BeforeEvent
}

// Func writes the event information into the logger
func (h *EventLogger) Func(
	item interface{},
	domain Hookable,
	info interface{},
) {
	evt := item.(Event)
	comp, ok := evt.Handler().(Component)
	if ok {
		h.Logger.Printf("%.10f, %s -> %s",
			evt.Time(), reflect.TypeOf(evt), comp.Name())
	} else {
		h.Logger.Printf("%.10f, %s", evt.Time(), reflect.TypeOf(evt))
	}
}
