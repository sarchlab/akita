# Pause and boundary inspection

Both engines process control requests at execution boundaries. For SerialEngine,
a boundary is outside an event and its before/after hooks. For ParallelEngine,
it is outside an existing batch, after all its workers and hooks have finished.
This does not change event ordering or the parallel scheduler's conflict model.
A wall-clock pause request does not select a deterministic virtual time; use
SerialEngine.RunUntil for that purpose.

## Pause and continue

`RequestPause()` submits a request and returns a `PauseRequest`. It does not wait
for the current handler or batch. Handlers and hooks may request a pause and then
return. They must not wait for acknowledgment themselves.

```go
request := engine.RequestPause()
// In the external controller, after any calling handler has returned:
if err := request.Wait(ctx); err != nil {
    return err
}
// The pause was acknowledged at a boundary. It persists until Continue.
```

`Pause() error` is shorthand for requesting a pause and waiting without a
deadline. `Continue() error` acknowledges resumption permission; it does not wait
for an event to execute. These blocking methods are for external controllers.
Repeated pause/continue requests are idempotent, not nested lock acquisitions.
Requests are applied in enqueue order. A concurrent Continue may therefore resume
an acknowledged pause; callers must use Inspect rather than assume a pause gives
them exclusive ownership of live model state.

`IsPaused()` is safe to call concurrently and reports acknowledged state. A
pending request alone does not make it true. Cancelling PauseRequest.Wait stops
that wait; it does not cancel or undo the pause. A request can be waited on more
than once.

`State()` reports `EngineRunning`, `EnginePausing`, `EnginePaused`, or
`EngineResuming`. A queued pause reports pausing until a boundary acknowledges it;
a queued continue reports resuming while dispatch is still paused. IsPaused is
true for paused and resuming. If opposing requests are queued, State reports the
first transition that changes the acknowledged state, in enqueue order. Repeated
requests and inspection alone do not introduce transitions. State reads do not
wait for events or inspection callbacks. Requests waiting to acquire the control
mutex have not been enqueued and do not yet change State.

These are dispatch-control states, not a run lifecycle: running also covers an
inactive engine with dispatch permission. Failure reporting remains through Run
and control errors. The monitor reads State directly, preserves the acknowledged
`paused` Boolean in its JSON response, and displays all four states. It disables
pause/continue during transitions. Very brief transitions may occur between polls.

## Inspection

`Inspect(ctx, func() error) error` executes a short read-only callback at a
boundary, whether running or paused. It does not change the pause state. Copy or
serialize the required state within the callback; do not return live pointers,
maps, or slices for the controller to read later.

```go
var snapshot bytes.Buffer
err := engine.Inspect(ctx, func() error {
    return json.NewEncoder(&snapshot).Encode(component.State)
})
if err != nil {
    return err
}
// Send snapshot.Bytes() to the client after inspection has finished.
```

Callbacks must not perform network I/O, mutate model state, or call engine
control methods (which would reenter the controller). Reading CurrentTime within
the callback is supported. Inspection callback errors and ordinary panics are
returned to the inspection caller; a read-only inspection failure does not
invalidate the simulation. This is not an extension-worker or mutation API.

Cancellation skips queued callbacks that have not started. It does not interrupt
an executing callback, which may finish after Inspect returns a cancellation
error. Do not read its output on error or reuse its buffer until it has finished.
Idle inspection executes synchronously and may wait for another idle inspection;
context cancellation cannot interrupt that mutex acquisition or callback.

The monitor's component, field, and virtual-time endpoints collect responses in
memory at the boundary, then write them to the HTTP client after leaving it. A
slow client does not prevent event dispatch or another control request.

## Inactive and failed runs

No background controller goroutine is created. While a run is active, its
goroutine serves control requests, including while waiting in the paused state.
While inactive, callers apply requests synchronously under the controller mutex,
excluding a new run. Application setup must still avoid concurrently mutating
model state or scheduling on a serial engine outside its execution goroutine.

Run completion and RunUntil's time limit settle pending requests before returning.
A pause processed at completion persists for the next run. Empty runs may return
without waiting for Continue. A failed run settles requests with its run error,
rejects subsequent controls/inspections, and does not report itself as paused.
Control requests cannot forcibly interrupt a nonreturning handler or callback.

## Migration and cost

Pause and Continue now return errors. Custom Engine implementations must also
provide RequestPause, IsPaused, State, and Inspect. Mock generation picks up these
methods. Existing scheduling and checkpoint interfaces are unchanged; control
requests and pause state are not serialized into checkpoints.

The ordinary serial event path checks one atomic pending flag. The control mutex
is used for requests, boundary work, and run start/end, never around each event.
ParallelEngine no longer holds a pause mutex across a batch or a user's pause.
It retains the existing queue and worker synchronization.
