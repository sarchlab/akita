package timing

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/hooking"
)

// EventLogger is an hook that prints the event information
type EventLogger struct {
	logger *log.Logger
}

// NewEventLogger returns a new LogEventHook which will write in to the logger
func NewEventLogger(logger *log.Logger) *EventLogger {
	h := new(EventLogger)

	h.logger = logger

	return h
}

type named interface {
	Name() string
}

// Func writes the event information into the logger
func (h *EventLogger) Func(ctx hooking.HookCtx) {
	if ctx.Pos != HookPosBeforeEvent {
		return
	}

	evt, ok := ctx.Item.(Event)
	if !ok {
		return
	}

	h.logger.Printf("%.10f, %s -> %s",
		evt.Time(), reflect.TypeOf(evt), evt.HandlerName())
}
