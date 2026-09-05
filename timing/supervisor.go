package timing

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// ErrClosed identifies operations rejected because healthy shutdown has begun.
var ErrClosed = errors.New("simulation is closed")

// FailureError describes the first failure of one simulation. Cause preserves the
// original panic value; Stack records where it was caught. A failed instance
// cannot be resumed. Diagnosis must not inspect mutable component state.
type FailureError struct {
	SimulationID string
	Operation    string
	Handler      string
	EventType    string
	Time         VTimeInPicoSec
	Cause        any
	Stack        []byte
	recordedBy   *Supervisor
}

func (f *FailureError) Error() string {
	return fmt.Sprintf("simulation %q failed during %s (handler %q, event %s, time %d): %v",
		f.SimulationID, f.Operation, f.Handler, f.EventType, f.Time, f.Cause)
}

func (f *FailureError) Unwrap() error {
	err, _ := f.Cause.(error)
	return err
}

// Supervisor owns a simulation's failure boundary and cooperative workers.
// Use Execute for setup and external mutations, and Go for extension workers.
// Callbacks must not recursively call Execute, Run, or Close. Workers must
// finish or respond to Context cancellation; arbitrary goroutines cannot be killed.
type Supervisor struct {
	opMu          sync.Mutex
	dispatchMu    sync.RWMutex
	stopped       atomic.Bool
	pauseSignal   atomic.Pointer[chan struct{}]
	mu            sync.Mutex
	id            string
	ctx           context.Context //nolint:containedctx // Owns the simulation lifetime, rather than a request context.
	cancel        context.CancelFunc
	failures      []*FailureError
	active        bool
	accepting     bool
	closing       bool
	closed        bool
	executed      bool
	workers       sync.WaitGroup
	workerCount   int
	workerChanged chan struct{}
	resume        chan struct{}
}

func NewSupervisor(id string) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{id: id, ctx: ctx, cancel: cancel, workerChanged: make(chan struct{}, 1)}
}

func (s *Supervisor) Context() context.Context { return s.ctx }

// SetID assigns diagnostic identity during setup, before starting workers.
func (s *Supervisor) SetID(id string) { s.mu.Lock(); defer s.mu.Unlock(); s.id = id }

// Err returns the first failure, or nil. The returned diagnostic is a copy.
func (s *Supervisor) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) == 0 {
		return nil
	}
	return cloneFailure(s.failures[0])
}

func cloneFailure(f *FailureError) *FailureError {
	c := *f
	c.Stack = append([]byte(nil), f.Stack...)
	return &c
}

// Failures returns independent snapshots, preserving secondary cleanup failures.
func (s *Supervisor) Failures() []*FailureError {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*FailureError, len(s.failures))
	for i, f := range s.failures {
		out[i] = cloneFailure(f)
	}
	return out
}

// Fail records an operational failure and cancels only this simulation.
func (s *Supervisor) Fail(operation string, cause any) { s.record(operation, nil, cause) }

func (s *Supervisor) record(operation string, evt Event, cause any) {
	if s.alreadyRecorded(cause) {
		return
	}
	f := &FailureError{Operation: operation, Cause: cause, Stack: debug.Stack(), recordedBy: s}
	if evt != nil {
		annotateFailure(f, evt)
	}
	s.mu.Lock()
	f.SimulationID = s.id
	s.failures = append(s.failures, f)
	s.stopped.Store(true)
	s.cancel()
	s.mu.Unlock()
}

// A failure returned through nested managed boundaries is one incident. Errors
// from another owner, and distinct later failures, must still be recorded.
func (s *Supervisor) alreadyRecorded(cause any) (recorded bool) {
	// A user-defined Unwrap method must not break the failure boundary.
	defer func() { _ = recover() }()
	err, ok := cause.(error)
	if !ok {
		return false
	}
	for err != nil {
		if failure, ok := err.(*FailureError); ok {
			return failure != nil && failure.recordedBy == s
		}
		// A joined error can contain an additional failure, so only suppress
		// ordinary single-cause wrapping of an already-recorded incident.
		wrapped, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = wrapped.Unwrap()
	}
	return false
}

// Check rejects operations on failed or closed instances by panicking. Managed
// callbacks contain this panic; callers outside a boundary own their recovery.
func (s *Supervisor) Check() {
	if !s.stopped.Load() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) > 0 {
		panic(cloneFailure(s.failures[0]))
	}
	if s.closed || s.closing {
		panic(ErrClosed)
	}
}

// Protect contains a callback on the current goroutine. Worker ownership and
// operation serialization belong to Execute/Go, not this low-level boundary.
func (s *Supervisor) Protect(operation string, evt Event, fn func() error) {
	defer func() {
		if p := recover(); p != nil {
			s.record(operation, evt, p)
		}
	}()
	if err := fn(); err != nil {
		s.record(operation, evt, err)
	}
}

// Execute runs a managed operation and joins every admitted worker, even after
// panic. Concurrent operations are rejected instead of waiting behind a run.
func (s *Supervisor) Execute(operation string, fn func() error) error {
	if !s.opMu.TryLock() {
		return errors.New("simulation already has an active operation")
	}
	defer s.opMu.Unlock()
	if err := s.begin(operation); err != nil {
		return err
	}
	s.Protect(operation, nil, fn)
	s.mu.Lock()
	s.accepting = false
	s.mu.Unlock()
	s.workers.Wait()
	s.mu.Lock()
	s.active = false
	s.mu.Unlock()
	return s.Err()
}

func (s *Supervisor) begin(operation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) > 0 {
		return cloneFailure(s.failures[0])
	}
	if s.closed || s.closing {
		return ErrClosed
	}
	s.active = true
	s.accepting = true
	if operation == "run" {
		s.executed = true
	}
	return nil
}

// Go starts a worker during a managed operation. A worker may launch children
// while admission remains open. Every worker must handle a rejected launch.
func (s *Supervisor) Go(operation string, fn func(context.Context) error) error {
	s.mu.Lock()
	if !s.accepting || s.closed || s.closing || len(s.failures) > 0 {
		s.mu.Unlock()
		return errors.New("simulation is not accepting workers")
	}
	s.workers.Add(1)
	s.workerCount++
	s.mu.Unlock()
	go func() { defer s.workerDone(); s.Protect(operation, nil, func() error { return fn(s.ctx) }) }()
	return nil
}

// Fresh reports whether execution has never begun. Restore is permitted only
// on a fresh instance; a failed restore is terminal, including preflight failure.
func (s *Supervisor) Fresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.executed && !s.closed && len(s.failures) == 0
}

// Pause prevents dispatch of subsequent events. Already admitted events finish.
func (s *Supervisor) Pause() {
	s.Check()
	s.mu.Lock()
	if s.resume == nil {
		s.resume = make(chan struct{})
		signal := s.resume
		s.pauseSignal.Store(&signal)
	}
	s.mu.Unlock()
	// An external pause waits until admitted callbacks have left their state.
	s.dispatchMu.Lock()
	s.dispatchMu.Unlock()
	s.Check()
}

func (s *Supervisor) Continue() {
	s.Check()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resume != nil {
		close(s.resume)
		s.resume = nil
		s.pauseSignal.Store(nil)
	}
}

func (s *Supervisor) awaitResume() bool {
	if signal := s.pauseSignal.Load(); signal != nil {
		select {
		case <-*signal:
		case <-s.ctx.Done():
			return false
		}
	}
	return !s.stopped.Load()
}

// Close cancels cooperative work, waits for the active operation, then contains
// cleanup. Cleanup must only release owned resources, never traverse failed models.
func (s *Supervisor) Close(cleanup func()) error {
	s.mu.Lock()
	if s.active && len(s.failures) == 0 {
		s.failures = append(s.failures, &FailureError{
			SimulationID: s.id, Operation: "termination during active operation",
			Cause: context.Canceled, Stack: debug.Stack(),
		})
	}
	s.closing = true
	s.stopped.Store(true)
	s.cancel()
	s.mu.Unlock()
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.Err()
	}
	s.closed = true
	s.mu.Unlock()
	s.Protect("cleanup", nil, func() error { cleanup(); return nil })
	return s.Err()
}

func annotateFailure(f *FailureError, evt Event) {
	// Broken event accessors must not escape while diagnosing the original panic.
	defer func() { _ = recover() }()
	f.EventType = fmt.Sprintf("%T", evt)
	f.Handler = evt.HandlerID()
	f.Time = evt.Time()
}

// State is safe to inspect without touching potentially invalid model state.
func (s *Supervisor) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failures) > 0 {
		return "failed"
	}
	if s.closed || s.closing {
		return "closed"
	}
	if s.active {
		return "running"
	}
	return "ready"
}

// Cancel terminates cooperative execution and makes failure terminal.
func (s *Supervisor) Cancel() { s.Fail("cancel", context.Canceled) }

func (s *Supervisor) workerDone() {
	s.mu.Lock()
	s.workerCount--
	s.mu.Unlock()
	select {
	case s.workerChanged <- struct{}{}:
	default:
	}
	s.workers.Done()
}

// waitForWork lets an extension worker schedule more events while the queue is
// empty, without blocking that worker's event-dependent progress.
func (s *Supervisor) waitForWork(wake <-chan struct{}) bool {
	s.mu.Lock()
	pending := s.workerCount > 0
	s.mu.Unlock()
	if !pending {
		// A producer may have scheduled its final event and exited after the
		// dispatcher observed an empty queue. Consume that wake before ending.
		select {
		case <-wake:
			return true
		default:
			return false
		}
	}
	select {
	case <-wake:
	case <-s.workerChanged:
	case <-s.ctx.Done():
		return false
	}
	return true
}

func (s *Supervisor) beginDispatch() bool {
	for s.awaitResume() {
		s.dispatchMu.RLock()
		if s.pauseSignal.Load() == nil && !s.stopped.Load() {
			return true
		}
		s.dispatchMu.RUnlock()
	}
	return false
}
func (s *Supervisor) endDispatch() { s.dispatchMu.RUnlock() }
