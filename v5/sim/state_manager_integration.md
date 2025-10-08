# State Manager Integration Guide

This document elaborates on how the proposed v5 state manager cooperates with existing simulation components, the `Simulation` struct, and both execution engines.

## Architectural Positioning

The state manager becomes a core dependency of `sim.Simulation`. Instead of the current `stateRegistry` map, the simulation now owns a `state.Manager` instance responsible for registering component state, providing transactional access, and handling serialization. The simulation exposes thin wrappers that delegate to the manager so callers do not depend on the manager’s internals.

```
type Simulation struct {
    ...
    stateMgr *state.Manager
}

func (s *Simulation) RegisterState(id string, def state.Definition) error
func (s *Simulation) LoadState(id string, dst any) error
func (s *Simulation) StageState(id string) (state.Transaction, error)
func (s *Simulation) BeginRound(ctx state.RoundContext) state.RoundGuard
```

The simulation also re-exports serialization helpers (`SaveState`, `LoadState`) so engines and orchestration tools can capture checkpoints without touching the manager directly.

## Component Interaction Model

Components already interact with the simulation through `sim.Component` and `sim.Driver` interfaces. The new manager preserves this contract by giving components two avenues for state access:

1. **Registration at build time.** During `builder.Build`, each component calls `simulation.RegisterState` with a schema describing its internal data. Simple structs or slices pass validation automatically; complex structures supply an adapter implementing the `state.Adapter` interface (snapshot, delta, apply, marshal/unmarshal).
2. **Per-cycle access through `StateAccessor`.** Replace the current `StateAccessor` with a richer interface that components receive during event processing:

   ```
   type StateAccessor interface {
       Load(id string, dst any) error
       Stage(id string) (state.Transaction, error)
   }
   ```

   - `Load` returns a defensive snapshot (copy or adapter-backed view) suitable for read-only operations within the tick.
   - `Stage` yields a `state.Transaction` handle that exposes mutation helpers prepared during registration. For simple structs the handle offers `Mutable() *T`, returning a copy-on-write pointer. For adapters, the handle exposes domain-specific methods such as `WriteRange`, `Push`, or `Pop`.

Components continue to express their behavior in terms of simulation cycles; the transaction handle simply accumulates changes until the engine closes the round.

## Round-Oriented Execution Model

Both the serial and parallel engines group work into **rounds**. A round is the execution of all events scheduled at the same simulation timestamp. Transactions created through `Stage` are therefore round-scoped: they become visible to other events only after the entire round finishes and the engine commits the collected mutations. This mirrors the existing event semantics—events in the same timestep should observe the pre-round state—while giving the manager a well-defined boundary for commit/rollback.

The manager tracks the currently active round via a `state.RoundContext` supplied by the engine. The context exposes:

```go
type RoundContext interface {
    ID() uint64            // Monotonic per engine, useful for logging and debugging
    Timestamp() sim.VTime  // Simulation time shared by all events in the round
}
```

Each transaction records the `RoundContext` that spawned it. A commit request that does not match the active round is rejected, preventing accidental leakage across time boundaries.

## Serial Engine Workflow

The serial engine still processes one event at a time, but it wraps all events sharing the same timestamp into a single round:

1. When the scheduler dequeues the first event for a timestamp, it calls `guard := stateMgr.BeginRound(ctx)` to create the round context and clear leftovers from the prior round.
2. For every event in that timestamp:
   - the engine provides a `StateAccessor` backed by the round context;
   - the component uses `Load`/`Stage` exactly as before, with all staged updates tracked against the round ID.
3. After the final event of the timestamp executes, the engine invokes `guard.FinishRound(ctx)`.
   - Because execution is serial, the commit should never encounter conflicts; if it does, the engine can treat it as a fatal bug.
4. The engine advances to the next timestamp, calling `BeginRound` again if more events exist.

`FinishRound` flushes every staged transaction atomically, ensuring that no event in the same round observes another event’s writes prematurely.

## Parallel Engine Workflow

Parallel engines schedule multiple components concurrently but still execute in rounds. The manager provides a `state.RoundGuard` to the engine when `BeginRound` is called:

```go
type RoundGuard interface {
    BeginTransaction(workerID string) state.CycleTxn
    StageTransaction(tx state.CycleTxn) error
    FinishRound(ctx RoundContext) error
    AbortRound(ctx RoundContext)
}
```

- `BeginRound` clears previous state, initializes bookkeeping for conflict detection, and returns a guard the engine must use for the remainder of the round.
- Each worker handling an event in the round calls `BeginTransaction` to obtain a `CycleTxn` bound to the round. The transaction records the worker ID, the state keys it touches, and their base versions.
- When a worker finishes, it submits the transaction via `StageTransaction`. The guard keeps the transaction pending until all workers finish or the engine determines the round should end early.
- `FinishRound` atomically commits the staged transactions, applying only those that are mutually compatible. If conflicts appear (e.g., two workers wrote the same state entry), the guard rejects the commit and reports the conflicting transaction IDs.
- The engine can retry conflicts by rescheduling the affected events in a subsequent round or, if progress is impossible, call `AbortRound` to discard all staged work for the timestamp.

## Interfaces Summary

- `Simulation`
  - `RegisterState(id string, def state.Definition) error`
  - `LoadState(id string, dst any) error`
  - `StageState(id string) (state.Transaction, error)`
  - `BeginRound(ctx state.RoundContext) state.RoundGuard`
  - `SaveState(w io.Writer, format Format) error`
  - `LoadState(r io.Reader, format Format) error`

- `state.Transaction`
  - `Mutable() any` for copy-on-write structs
  - Adapter-specific methods via generated interfaces (e.g., `MemoryDelta`, `BufferDelta`)
  - `Commit()`/`Discard()` handled internally by the manager; external callers only receive the handle.

- `state.RoundContext`
  - `ID() uint64`
  - `Timestamp() sim.VTime`

- `state.RoundGuard`
  - `BeginTransaction(workerID string) state.CycleTxn`
  - `StageTransaction(tx state.CycleTxn) error`
  - `FinishRound(ctx RoundContext) error`
  - `AbortRound(ctx RoundContext)`

- `StateAccessor` (supplied to component handlers)
  - `Load(id string, dst any) error`
  - `Stage(id string) (state.Transaction, error)`

This structure keeps existing component logic largely intact while centralizing state safety within the manager. Serial execution remains fast thanks to low-overhead staging, and parallel execution gains the conflict detection needed for deterministic commits.
