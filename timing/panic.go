package timing

import (
	"fmt"
	"runtime/debug"
)

// PanicError reports an ordinary Go panic caught while executing an engine.
// The failed engine must be discarded; recovery does not roll back model state.
// Cause preserves the panic value, including error values for errors.Is/As.
type PanicError struct {
	Cause     any
	Stack     []byte
	EventType string
	Handler   string
	Time      VTimeInPicoSec
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("simulation panicked (handler %q, event %s, time %d): %v",
		e.Handler, e.EventType, e.Time, e.Cause)
}

func (e *PanicError) Unwrap() error {
	err, _ := e.Cause.(error)
	return err
}

func newPanicError(cause any, event Event) *PanicError {
	err := &PanicError{Cause: cause, Stack: debug.Stack()}
	if event != nil {
		err.EventType = fmt.Sprintf("%T", event)
		annotatePanic(err, event)
	}
	return err
}

func annotatePanic(err *PanicError, event Event) {
	// A broken event accessor must not replace the original panic.
	defer func() { _ = recover() }()
	err.Handler = event.HandlerID()
	err.Time = event.Time()
}
