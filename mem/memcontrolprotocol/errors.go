package memcontrolprotocol

// Protocol-defined Error strings on ControlRsp.
const (
	// ErrUnsupported means the component does not implement the verb.
	ErrUnsupported = "unsupported"

	// ErrMustBePausedOrDrained means Invalidate or Flush was issued
	// while enabled and the requested operation requires Pause or Drain.
	ErrMustBePausedOrDrained = "must be paused or drained"

	// ErrCanceledByReset answers a Hold that was not acknowledged yet when
	// the component was reset. The pages are not held.
	ErrCanceledByReset = "canceled by reset"

	// ErrInvalidRange means the Range of the request is empty or reversed
	// (its End is not greater than its Start), or is set without HasRange.
	ErrInvalidRange = "invalid address range"

	// ErrAmbiguousAddressing means the request has both HasRange and a
	// nonempty Addresses.
	ErrAmbiguousAddressing = "both range and addresses"
)
