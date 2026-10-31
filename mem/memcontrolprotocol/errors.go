package memcontrolprotocol

// Protocol-defined Error strings on ControlRsp.
const (
	// ErrUnsupported means the component does not implement the verb.
	ErrUnsupported = "unsupported"

	// ErrMustBePausedOrDrained means Invalidate or Flush was issued
	// while the component was Enabled. The caller must Pause or Drain
	// first.
	ErrMustBePausedOrDrained = "must be paused or drained"

	// ErrCanceledByReset answers a Hold that was not acknowledged yet when
	// the component was reset. The pages are not held.
	ErrCanceledByReset = "canceled by reset"
)
