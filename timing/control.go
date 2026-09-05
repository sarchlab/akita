package timing

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// PauseRequest acknowledges a requested pause once execution reaches a boundary.
// Multiple callers may Wait. Cancellation stops waiting, not the pause request.
type PauseRequest interface {
	Wait(context.Context) error
}

type pauseRequest struct {
	done chan struct{}
	err  error
}

// Wait waits for acknowledgment or cancellation. It must not be called from an
// event handler or hook: acknowledgment requires that callback to finish first.
func (r *pauseRequest) Wait(ctx context.Context) error {
	select {
	case <-r.done:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type controlKind uint8

const (
	controlPause controlKind = iota
	controlContinue
	controlInspect
)

type controlRequest struct {
	kind    controlKind
	inspect func() error
	result  *pauseRequest
}

// engineControl belongs to the execution goroutine while running. The mutex
// protects requests and transfers ownership to callers while Run is inactive.
// It is never held across an event or while waiting for Continue.
type engineControl struct {
	mu      sync.Mutex
	changed *sync.Cond
	pending atomic.Bool // requests pending, or paused
	paused  atomic.Bool // acknowledged state, not requested state
	running bool
	failure error
	queue   []controlRequest
}

func newEngineControl() *engineControl {
	c := &engineControl{}
	c.changed = sync.NewCond(&c.mu)
	return c
}

// RequestPause requests a pause without waiting for the current event or batch.
// Handlers and hooks may call it, but must return before waiting on the result.
func (c *engineControl) RequestPause() PauseRequest {
	return c.submit(controlRequest{kind: controlPause})
}

// Pause requests and waits for a pause. Use RequestPause from handlers/hooks or
// when a cancellable wait is needed. Repeated pauses are idempotent.
func (c *engineControl) Pause() error {
	return c.RequestPause().Wait(context.Background())
}

// Continue resumes event dispatch. It is idempotent and is for external callers.
func (c *engineControl) Continue() error {
	return c.submit(controlRequest{kind: controlContinue}).Wait(context.Background())
}

// IsPaused reports the last acknowledged pause state. It is safe during Run.
func (c *engineControl) IsPaused() bool { return c.paused.Load() }

// Inspect runs a read-only callback between events (between joined batches in a
// parallel engine), without changing pause state. The callback must copy or
// serialize its result; it must not expose live references, perform network I/O,
// or call engine control methods. CurrentTime may be read inside the callback.
// When Run is inactive, inspection is synchronous and excludes a new Run.
// Cancellation skips a callback that has not started; it cannot interrupt one
// already executing. Callback errors/panics are returned to the inspection caller.
func (c *engineControl) Inspect(ctx context.Context, inspect func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.submit(controlRequest{
		kind: controlInspect, inspect: func() error { return inspectAtBoundary(ctx, inspect) },
	}).Wait(ctx)
}

func (c *engineControl) submit(req controlRequest) *pauseRequest {
	req.result = &pauseRequest{done: make(chan struct{})}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		c.queue = append(c.queue, req)
		c.pending.Store(true)
		c.changed.Signal()
	} else {
		c.apply(req)
		c.pending.Store(c.paused.Load())
	}
	return req.result
}

func (c *engineControl) apply(req controlRequest) {
	defer close(req.result.done)
	if c.failure != nil {
		req.result.err = c.failure
		return
	}
	switch req.kind {
	case controlPause:
		c.paused.Store(true)
	case controlContinue:
		c.paused.Store(false)
	case controlInspect:
		req.result.err = req.inspect()
	}
}

func inspectAtBoundary(ctx context.Context, inspect func() error) (err error) {
	defer func() {
		if cause := recover(); cause != nil {
			err = newPanicError(cause, nil)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return inspect()
}

func (c *engineControl) begin() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return c.failure
	}
	if c.running {
		return fmt.Errorf("timing: Run is already active")
	}
	c.running = true
	return nil
}

func (c *engineControl) drain() {
	for _, req := range c.queue {
		c.apply(req)
	}
	c.queue = nil
}

// boundary is entered only when pending is set, keeping the ordinary event
// path to one atomic load. Cond.Wait releases mu so paused runs serve controls.
func (c *engineControl) boundary() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		c.drain()
		if !c.paused.Load() {
			c.pending.Store(false)
			return
		}
		c.changed.Wait()
	}
}

// end settles requests even when the queue empties, RunUntil reaches its limit,
// or execution panics. It runs after engine recovery has assigned the run error.
func (c *engineControl) end(err *error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	if *err != nil {
		c.failure = *err
		c.paused.Store(false)
	}
	c.drain()
	c.pending.Store(c.paused.Load())
}
