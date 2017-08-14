package util

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// EventIssueWidthLogger logs how many events can be issued in parallel
type EventIssueWidthLogger struct {
	LogHookBase
	now   core.VTimeInSec
	round int
	count int
}

// NewEventIssueWidthLogger returns a new LogEventHook which will write in to the logger
func NewEventIssueWidthLogger(logger *log.Logger) *EventIssueWidthLogger {
	h := new(EventIssueWidthLogger)
	h.Logger = logger
	h.Logger.Printf("round, time, width\n")
	return h
}

// Type always return the type of a Event
func (h *EventIssueWidthLogger) Type() reflect.Type {
	return reflect.TypeOf((core.Event)(nil))
}

// Pos of a PrintEventHook suggests that it should be called before the
// event handling.
func (h *EventIssueWidthLogger) Pos() core.HookPos {
	return core.BeforeEvent
}

// Func writes the event information into the logger
func (h *EventIssueWidthLogger) Func(
	item interface{},
	domain core.Hookable,
	info interface{},
) {
	evt := item.(core.Event)
	if evt.Time() != h.now {
		h.Logger.Printf("%d, %.10f, %d\n", h.round, h.now, h.count)
		h.now = evt.Time()
		h.round++
		h.count = 0
	}
	h.count++
}
