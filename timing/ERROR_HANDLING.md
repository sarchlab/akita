# Engine errors and model panics

`Handler.Handle(Event)` has no error result. A model invariant violation may
panic; an expected simulated failure belongs in a response, status, or event.
Backpressure, lookup misses, and no work retain their ordinary result types.

`SerialEngine.Run`, `SerialEngine.RunUntil`, and `ParallelEngine.Run` return
errors. An ordinary Go panic during execution, including before/after hooks,
becomes a `*timing.PanicError`. It retains the original cause and stack, plus
best-effort event/handler/time details. Error-valued causes support
`errors.Is` and `errors.As`. A handler panic skips its after-event hook.

Check the execution error and discard the failed engine and its model state.
Recovery does not roll back mutations. A failed engine rejects subsequent runs
and scheduling; a failed serial engine also rejects checkpoint save/load.
Independent engines can continue executing in the same process.

```go
if err := engine.Run(); err != nil {
    // Report this engine's failure and discard its model state.
    return err
}
```

Serial recovery is installed once around the run, on its existing goroutine.
Parallel recovery runs inside the existing workers as well as the dispatcher.
Parallel failure wakes workers waiting to borrow a queue, and Run joins started
workers before returning. Queues, event ordering, concurrency, and pause/resume
contracts are otherwise unchanged. There is no extension-worker API or new
serial scheduling concurrency.

Containment covers ordinary Go panics during engine execution. Construction,
direct handler calls, and caller-created goroutines remain the caller's
responsibility. Recovery cannot stop a nonreturning handler or contain fatal
runtime failures, `os.Exit`, or unsafe/native corruption. It does not isolate
host-shared mutable resources or change global ID/checkpoint ownership.

`mem.Storage.Read(address, length)` returns `[]byte`; `Write(address, data)`
has no result. Empty, overflowing, or out-of-capacity accesses panic before
allocating storage units or changing stored bytes. The existing valid-range
behavior is unchanged, including accesses ending exactly at capacity.

Existing external I/O, validation, JSON, and checkpoint error signatures remain
unchanged. Setup/finalization error handling, reader/recorder normalization, and
broader ownership or restore policy are separate work under RFC #478.

## Migration

- Remove `error` from `Handle` declarations and remove their `return nil`.
  Express modeled failures through the model protocol; panic for violated
  invariants. Always inspect the error from `Run`/`RunUntil`.
- Replace `data, err := storage.Read(...)` with `data := storage.Read(...)`.
  Call `storage.Write(...)` directly. Invalid ranges are programmer errors;
  handle a resulting execution failure at the engine boundary.
