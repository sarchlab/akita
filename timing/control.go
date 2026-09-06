package timing

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// EngineState reports dispatch permission and pending pause/resume transitions.
// It does not report whether Run is active or whether a simulation has failed.
type EngineState uint32

const (
	EngineRunning  EngineState = iota // Dispatch is permitted, including while idle.
	EnginePausing                     // A pause is queued; the boundary has not acknowledged it.
	EnginePaused                      // Dispatch is paused at a boundary.
	EngineResuming                    // A continue is queued; dispatch remains paused.
)

func (s EngineState) String() string {
	switch s {
	case EngineRunning:
		return "running"
	case EnginePausing:
		return "pausing"
	case EnginePaused:
		return "paused"
	case EngineResuming:
		return "resuming"
	default:
		return "unknown"
	}
}

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
	pending atomic.Bool   // requests pending, or paused
	state   atomic.Uint32 // published state; readable during inspection
	paused  bool          // acknowledged dispatch state, protected by mu
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
func (c *engineControl) IsPaused() bool {
	state := c.State()
	return state == EnginePaused || state == EngineResuming
}

// State reports the acknowledged state and the next queued transition. It is
// safe during Run and does not wait for an event, batch, or inspection callback.
func (c *engineControl) State() EngineState { return EngineState(c.state.Load()) }

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
		c.publishState()
		c.changed.Signal()
	} else {
		c.apply(req)
		c.pending.Store(c.paused)
	}
	return req.result
}

func (c *engineControl) apply(req controlRequest) {
	defer close(req.result.done)
	defer c.publishState()
	if c.failure != nil {
		req.result.err = c.failure
		return
	}
	switch req.kind {
	case controlPause:
		c.paused = true
	case controlContinue:
		c.paused = false
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
	for len(c.queue) > 0 {
		req := c.queue[0]
		c.queue[0] = controlRequest{}
		c.queue = c.queue[1:]
		c.apply(req)
	}
	c.queue = nil
}

// publishState runs under mu. Report the first queued request that changes
// acknowledged dispatch permission; inspections and idempotent controls do not
// create transitions. FIFO order also applies to opposing queued requests.
func (c *engineControl) publishState() {
	state := EngineRunning
	if c.paused {
		state = EnginePaused
	}
	if c.failure == nil {
		for _, req := range c.queue {
			if req.kind == controlPause && !c.paused {
				state = EnginePausing
				break
			}
			if req.kind == controlContinue && c.paused {
				state = EngineResuming
				break
			}
		}
	}
	c.state.Store(uint32(state))
}

// boundary is entered only when pending is set, keeping the ordinary event
// path to one atomic load. Cond.Wait releases mu so paused runs serve controls.
func (c *engineControl) boundary() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		c.drain()
		if !c.paused {
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
		c.paused = false
	}
	c.publishState()
	c.drain()
	c.pending.Store(c.paused)
}
