# Error handling and simulation ownership in v5

Akita supports several independent simulations in one Go process. Model
precondition violations panic. The owning simulation contains ordinary Go
panics and returns a `*timing.FailureError` from its managed boundary. Failure
is terminal: containment does not roll back model state or allow resuming it.

## Managed boundaries

Use `Builder.Build` callbacks for component construction, `Simulation.Setup`
for subsequent setup or externally initiated mutations while execution is
stopped, and `Simulation.Run` or the engine's `Run`/`RunUntil` for execution.
All return errors. Check those errors before using the result.

```go
sim, err := simulation.MakeBuilder().WithoutMonitoring().Build(
    func(sim *simulation.Simulation) error {
        // Build components, register handlers, and schedule initial events here.
        // A panic or returned error fails only this simulation.
        return nil
    },
)
if err != nil {
    return err // failed setup already released its owned resources
}
if err := sim.Run(); err != nil {
    _ = sim.Terminate() // cleanup cannot escape as a panic
    return err
}
return sim.Terminate() // final recording errors matter too
```

Standalone serial and parallel engines expose `Supervisor()` for the same
managed setup/worker/cleanup contract. Extensions start workers with
`sim.Go(name, func(context.Context) error)` inside an active managed operation.
A worker must finish or respond to cancellation. Handle rejected worker
launches; do not start unmanaged goroutines that touch simulation state.
`Run` waits for owned workers and processes events they schedule before
returning. `Cancel` stops cooperative work and records terminal failure.

Do not recursively call `Run`, `Setup`, or checkpoint operations from their
callbacks. Concurrent managed operations return an error. `Pause` and
`Terminate` are host operations: do not call them from an event callback or
owned worker, because they wait for that work to settle. External `Pause`
waits for admitted event callbacks to leave model state; `Continue` resumes.
It is still the host's responsibility to stop other extension producers before
inspecting their state or saving a checkpoint.

Direct access to public model state, direct handler calls, and unmanaged
host callbacks remain the caller's responsibility. Akita cannot intercept
arbitrary Go assignments. Use a managed boundary to contain their failures.

## Failure and diagnostics

`State()` reports `ready`, `running`, `failed`, or `closed`. `ready` means no
managed operation is active; it does not imply that a previously executed
instance is fresh enough to restore. `failed` takes precedence over `closed`.
`Err()` returns the first failure. `Failures()` preserves later worker and
cleanup failures as diagnostic snapshots, including stack traces. An error
panic remains available through `errors.Is`/`errors.As`; string and other panic
values are retained in `FailureError.Cause`.

After failure, inspect these diagnostics, the simulation ID, warnings, and
engine time, or call `Terminate`. Do not inspect or mutate component state.
Further run/setup/checkpoint operations return errors. Direct scheduling or
resuming violates the failed-instance precondition and panics; using it inside
`Setup` returns the already recorded failure. The monitor rejects model
inspection and mutation on failed instances. `timing.ErrClosed` identifies an
operation rejected during healthy shutdown. An in-flight monitor inspection
may encounter it while releasing its pause; that specific condition produces
a diagnostic without invalidating completed results. Other panics still fail
the simulation, including panics already unwinding when inspection cleanup runs.

Recovery covers ordinary Go panics in managed setup, hooks, handlers, and
workers. It cannot contain fatal runtime failures such as out-of-memory,
unsafe/native memory corruption, `os.Exit`, or kill a noncooperative goroutine.
A host needing isolation against those failures still needs separate processes.

## IDs, tracing, and resources

Every engine owns its ID generator. Use `sim.GetIDGenerator()`,
`engine.IDGenerator()` on concrete engines, or `timing.IDsFor(engine)` through
an interface, and pass that source into event/message construction. Components
expose `IDGenerator()` for their middleware. IDs need only be unique within one
simulation; two independent simulations may generate the same numeric IDs.
Tracing task associations belong to that sequence and are checkpointed with it.
Custom ID generators must implement the full concurrent association contract.

The process-global ID generator, reset, and generator-selection APIs have been
removed. Never share a generator or mutable model resources between independent
simulations. Shared immutable configuration is fine. Hosts supplying mutable
resources or database handles explicitly take responsibility for isolation;
`NewDataRecorderWithDB` transfers closing responsibility to the recorder.

Recorders no longer register process-wide exit callbacks. Always finalize
explicitly. Output creation rejects an existing database instead of deleting
it, so two simulations cannot overwrite each other's recording. Use distinct
output paths, and choose a new path or explicitly remove a discarded recording
before retrying.

## Optional features and output validity

An unavailable monitor or source archive produces a warning through
`Warnings()` and logging. Warnings are recorded in execution metadata during
successful finalization. Missing source may prevent source browsing, but it
does not invalidate numerical results. Unexpected panics or state corruption
are never treated as optional degradation.

A database write, requested trace, final flush, index build, or close failure
fails the simulation. Partial recordings may remain for diagnosis. An end
timestamp alone is not evidence of valid output. Successful `Terminate`
publishes `<output-base>.complete` only after recording and database close
succeed. Absence of that marker means completion has not been established;
copy it alongside completed recordings. The returned termination error remains
the authoritative signal to the running host.

Recorder I/O methods return errors. `datarecording.MustRecord(err)` adapts an
I/O failure to a panic inside a managed model/tracing callback. Standalone
recording clients should handle errors directly. A failed flush retains its
batch for retry; retry `Flush`, not the insertion that triggered automatic
flushing. `Close` always releases the database and is final, even on failure.

## Fresh restore and validation

Rebuild a new compatible simulation and load its checkpoint before execution.
Do not rewind an existing run. A target may attempt restore only once. Any
restore failure invalidates the entire target, including failures during
preflight. Discard it and construct another target to retry. Earlier entities
may already have changed; global transactional rollback is not provided.
Other simulations' IDs, resources, and output remain untouched.

Container geometry, storage addresses, and event references are validated
before local assignment. Port restore stages both buffers together. These local
checks reduce partial mutation but do not make arbitrary entity serializers
transactional. Save checkpoints only while the simulation and its producers
are stopped. Failed instances cannot produce valid checkpoints.

## Interface migration

- `Handler.Handle(Event)` has no error result. Normal modeled failures remain
  protocol responses; backpressure and lookup misses remain ordinary results.
- `Storage.Read(address, length)` returns `[]byte`; `Storage.Write(address,
  data)` has no result. Empty, overflowing, and out-of-capacity ranges panic
  before allocating storage units or modifying bytes.
- `Builder.Build(...setupCallbacks)` returns `(*Simulation, error)`.
  `Simulation.Terminate()` returns an error.
- `MakeEventBase`, `MakeTickEvent`, and `MakeTimerFiredEvent` take the owning
  `IDGenerator` as their first argument. Tracing domains implement `IDSource`.
- `NewDataRecorder` and `NewReader` return `(value, error)`.
  `CreateTable`, `InsertData`, and `Flush` return errors.
- Monitor start/stop and replay-server creation/start/stop return errors.
  Trace-reader queries return errors instead of panic or partial success.
- JSON, codecs, checkpoint serializers, explicit validators, external I/O,
  request validation, and provider operations retain their error contracts.

Run `go run ./examples/failureisolation` for a two-simulation example. It reports
one failed event in the first simulation and three completed events in its peer,
then exits successfully because the injected failure was contained as expected.
