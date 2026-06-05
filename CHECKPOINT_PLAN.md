# Checkpointing Plan

## Goal

Phase A is merged. The repository now has a runtime inventory of named entities,
but no persistence code. Phase B should turn that inventory into a transparent
checkpoint/restore system for `SerialEngine`.

The correctness oracle is:

```text
run from t0 to done == run from t0 to checkpoint, restore, then run to done
```

This must hold when the checkpoint is taken mid-run, with queued events,
scheduled ticks, messages in ports, connection state, component state, and shared
resources all live.

## Current Baseline

The merged Phase A baseline is:

- `simulation.Simulation` owns typed registries for components, ports,
  connections, and resources, plus a private flat entity inventory.
- Engine and ID generator are registered singleton entities named `Engine` and
  `IDGenerator`.
- Components are `modeling.Component[Spec, State, Resources]` or
  `modeling.EventDrivenComponent[Spec, State, Resources]`.
- Component `Spec` and `Resources` are private construction-time fields exposed by
  accessors. `State` is public so middleware can mutate it.
- Shared resources are registered explicitly by setup. `ResourceOwner`
  auto-registration is gone.
- Ports are discovered during component registration. Existing messaging
  components are adapted with reflection because `Ports() []messaging.Port` is
  not assignable to `[]simulation.Port`.
- `messaging.Msg` and `timing.Event` are immutable **value** types, constructed
  once and used once. `Msg.Meta()` returns `MsgMeta` by value; `MsgMeta` carries
  no task-ID fields. Tracing keeps receiver task IDs in a side-table keyed by
  (domain, message ID), so messages are never mutated in flight.
- Ports use the `CanSend`/`CanDeliver` convention: `Send`/`Deliver` panic on
  overflow, so callers check capacity first.
- `queueing.Buffer` and `queueing.Pipeline` are encapsulated value types and now
  serialize their full contents through `MarshalJSON`/`UnmarshalJSON`.
- The old quiescent `Save`/`Load` code was removed. Phase B starts fresh.

There is currently no public `GetStateByName`, no public `Entities()` accessor,
and no checkpoint archive writer/loader.

## Non-Goals

- No cross-version migration. A checkpoint is consumed by the same executable that
  produced it.
- No `ParallelEngine` checkpoint support in Phase B. Save/load must reject it.
- No serialization of observers or setup choices: hooks, tracers, monitors, data
  recorders, visualizers, output handles, and HTTP state are rebuilt or omitted.
  This includes tracing's global receiver-task-ID side-table: it is observer
  state, so it is not checkpointed and traces do not span a checkpoint boundary.
  One caveat: that table generates task IDs from the global ID generator when
  tracing is active, so it perturbs the ID-generator counter that *is*
  checkpointed. For a deterministic resume oracle (byte-exact counter), run
  checkpointable simulations with tracing off, or treat tracing's ID consumption
  as outside the determinism guarantee.
- No arbitrary pointer-graph checkpointing. Runtime references cross the boundary
  by stable names, IDs, or explicit typed payloads.
- No rollback guarantee after a payload-level load failure. Load should check the
  build identity and coverage before mutation, but callers should load into a
  freshly rebuilt simulation and discard it on failure.

## Full Support Definition

Phase B is complete when Akita can:

- Save a single archive while a serial simulation is paused or otherwise stopped
  outside an event handler.
- Restore into a freshly rebuilt simulation with the same topology and build
  identity.
- Preserve engine time, event queues, event ordering, ID generator state,
  component state, tick scheduler state, event-driven wakeup guards, port
  buffers, connection runtime state, queueing containers, and registered shared
  resources.
- Decode polymorphic `messaging.Msg` and `timing.Event` values through explicit
  registries.
- Fail loudly on missing, extra, duplicated, unknown, unsupported, or mismatched
  entries.
- Pass integration tests that compare uninterrupted execution against
  checkpoint/resume execution.

## Core Decisions

### Setup rebuilds shape; checkpoint restores runtime

Setup code rebuilds components, ports, resources, connection topology, handlers,
and observers. The checkpoint restores only values that can change future
simulation behavior.

Saved runtime values:

- Engine current time and queued events.
- ID generator kind and next counter.
- Component `State`.
- Ticking scheduler private state.
- Event-driven pending wakeup guard.
- Port incoming and outgoing buffers.
- Connection runtime state, such as round-robin cursors.
- Shared resources, such as memory contents and page tables.

Compared, not blindly restored:

- Component `Spec` (via a spec hash carried in the component's own payload).
- Resource shape metadata, such as memory capacity or page size.
- Rebuilt topology (via the saved-vs-rebuilt name-set comparison).

Not restored:

- `Resources` references; setup reinjects them.
- Port objects and connection plug-in lists; setup rebuilds them.
- Hooks, monitors, tracers, and recorders.

### The event queue is the source of truth for future work

Pending work exists in several places: the engine event queue, tick scheduler
guards, event-driven wakeup guards, and messages in port buffers. To avoid double
scheduling, the restored event queue is authoritative.

Load should restore raw port buffers and scheduler/wakeup guards without calling
behavioral APIs such as `Send`, `Deliver`, `NotifyRecv`, `NotifyPortFree`,
`TickNow`, or `TickLater`. Post-load hooks may validate consistency, but they
must not schedule duplicate work unless the design explicitly proves the restored
queue lacks that work.

### Strict is the default

Strict load fails when:

- The build identity does not match.
- A saved entity is not present in the rebuilt simulation.
- A rebuilt runtime entity is missing from the checkpoint.
- An archive entry is neither the build identity nor a known entity payload.
- A payload type tag cannot be decoded.
- A rebuilt spec or topology does not match the saved compatibility hash.

An opt-in relaxed mode for extra entries can be considered later, but Phase B
should ship strict-only first.

## Archive Format

Use one `tar.gz` archive written through a temporary path and renamed on success:

```text
checkpoint.tar.gz
  build_id
  entities/<encoded-name>
```

There is no manifest. The only central datum is the build identity, written as a
lone `build_id` entry; the set of `entities/<name>` payload files is itself the
inventory. Each entity owns its own payload bytes (JSON or binary). Archive paths
are escaped so entity names never become direct filesystem paths, and entries are
sorted by name for reproducibility.

Coverage is checked by the simulation, which compares the saved name set against
its rebuilt entity inventory. Anything finer-grained (per-entity spec hash,
content checksum, type tag) is pushed into the entity's own payload if and when
that entity needs it, never carried centrally. Cross-binary divergence is caught
by the build identity; topology divergence from the same binary (e.g. a different
GPU count from a parameterized setup) is caught by the name-set comparison.

## API Shape

Add checkpoint orchestration on `simulation.Simulation`:

```go
func (s *Simulation) SaveCheckpoint(path, buildID string) error
func (s *Simulation) LoadCheckpoint(path, buildID string) error
```

`buildID` overrides the build identity (mainly for tests); pass `""` to use the
default. `SaveCheckpoint` is low-level: it assumes the simulation uses a
`SerialEngine` and is stopped outside an event handler.

Use a small leaf package, `checkpoint`, for the archive read/write helpers, the
build identity, and the entity capability interface. That avoids import cycles
while letting `modeling`, `messaging`, `timing`, `mem`, and `simulation` share one
interface:

```go
type Checkpointable interface {
    SaveCheckpoint(w io.Writer) error
    LoadCheckpoint(r io.Reader) error
}
```

An entity owns its own bytes, so this is the only interface the foundation needs.
A load context for cross-entity lookups (by name, handler ID, message ID) and a
post-load hook are added later, by extending the load signature when an entity
actually needs them — there are no implementers yet, so that change is free. The
simulation infers entity kind from its typed registries, so the core entity
interface stays `Name() string`. Every registered runtime entity must implement
`Checkpointable`, or save/load fails loudly.

## Triggering (future work)

Phase B saves at an explicit boundary: the caller holds a `SerialEngine` that is
stopped outside an event handler, then calls `SaveCheckpoint`. Automatic triggers
and the engine coordination they need are deferred to a later phase. The intended
shape, kept short here so it does not drive the foundation design:

- **Use cases**: wall-clock interval (fault tolerance), virtual-time interval
  (deterministic snapshots), semantic milestone (e.g. kernel completion), and
  operator/manual request.
- **Determinism**: virtual-time and semantic triggers should be handled inline by
  the engine's between-events check, so they need no concurrency. Only wall-clock
  triggers are inherently real-time and would need an asynchronous coordinator
  plus a `SerialEngine` boundary-pause handshake (let the running handler return,
  pause before popping the next event, save, then resume or stop). A handler must
  only *request* a checkpoint and return; it can never block waiting for its own
  boundary.
- **Placement**: a checkpoint requester is control-plane wiring. Put it at the
  behavior boundary that knows the trigger (a middleware or wrapper), never in the
  generic `modeling.Component` or in `Spec`/`State`/`Resources`. On restore, setup
  rebuilds it like any other control handle; it is not serialized.

None of this is needed for the foundation or quiescent milestones, which save only
at an already-stopped boundary.

## Implementation Work

### 0. Preflight cleanup

- Fix documentation that still describes removed APIs or default JSON support
  that no longer exists (`simulation/README.md`, `queueing/README.md`, and the
  checkpoint-related parts of `doc/component_guide.md`).
- Fix checkpoint-adjacent runtime invariants before using them as test oracles:
  `datamover` must not pop a control request while busy, storage must reject
  invalid unit sizes, and queueing should enforce `Accept`/capacity invariants.
- Add focused tests for deterministic entity names and globally unique singleton
  names.

### 1. Checkpoint package and archive

- Add the `Checkpointable` interface, entity-path escaping, the build-identity
  helper, and the archive read/write helpers (build_id entry plus per-entity
  payloads, sorted and reproducible).
- Compute build identity from Go build info plus VCS revision and dirty status
  when available; provide a `buildID` override for tests.
- Add archive tests for sorted output, unknown-entry rejection, empty/duplicate
  guards, and round trips.

### 2. Simulation orchestration

- Implement `SaveCheckpoint` as a flat deterministic loop over the entity
  inventory: ask each entity for its payload bytes and write the archive with the
  build identity.
- Implement `LoadCheckpoint` as staged work:
  1. Read the archive and check the build identity.
  2. Validate coverage: saved names == rebuilt names.
  3. Hand each entity its payload bytes (no behavioral calls).
  4. Run post-load validation hooks if/when entities implement them.
- Save only under `SerialEngine`. Detecting "inside an event handler" needs engine
  support and is deferred with triggering; until then `SaveCheckpoint` is a
  caller-discipline boundary.
- Keep the flat inventory private; tests validate through public registries and
  save/load behavior.

### 3. Generic component checkpointing

- Implement checkpoint methods for `modeling.Component[S, T, R]`.
- Save a JSON DTO containing name, type, spec hash, state, and ticking scheduler
  snapshot.
- On load, compare the rebuilt spec hash and unmarshal only `State` and scheduler
  runtime fields.
- Do not serialize `Resources`; setup restores those references.
- Run `ValidateSpec` and `ValidateState` as part of checkpoint validation, with
  explicit exemptions for types that implement custom JSON.

### 4. Event-driven component checkpointing

- Implement checkpoint methods for `modeling.EventDrivenComponent[S, T, R]`.
- Save name, type, spec hash, state, and `pendingWakeup`.
- Restore `pendingWakeup` without scheduling a new event.
- Validate that restored `TimerFiredEvent` queue entries and the wakeup guard are
  consistent.

### 5. Queueing containers

- Add explicit JSON DTOs or `MarshalJSON`/`UnmarshalJSON` implementations for
  `queueing.Buffer[T]` and `queueing.Pipeline[T]`.
- Preserve buffer name, capacity, FIFO contents, pipeline width, stage count, lane,
  stage, item, and cycle-left fields.
- Do not serialize `HookableBase`; hooks are observers and are rebuilt.
- Validate capacities, widths, lane bounds, stage bounds, and item count on load.

### 6. Message registry

- Add a `messaging` codec registry for `messaging.Msg` values.
- Encode messages as typed payloads:

  ```go
  type TypedPayload struct {
      Type    string          `json:"type"`
      Payload json.RawMessage `json:"payload"`
  }
  ```

- Use `%T`-derived keys for same-binary checkpoints. Messages are value types, so
  register the value (`messaging.RegisterMsg(mem.ReadReq{})`); the codec
  reconstructs the same value (or pointer) form from the tag.
- Each message-defining package registers its types in an `init()` (see
  `mem/msgcodec.go`). A forgotten registration fails loud at checkpoint time.
- Because messages are now immutable value snapshots that no longer carry
  task-ID fields, default JSON captures the whole message — the old concern about
  in-transit metadata mutation or non-serializable task IDs no longer applies. A
  transient `Info interface{}` field stays `json:"-"`; it is data-plane scratch
  that is empty for a buffered message.

### 7. Port checkpointing

- Implement checkpoint support in `messaging.defaultPort`.
- Save incoming and outgoing capacities plus buffer contents through the message
  registry.
- Restore buffers directly with `Buffer.Restore`, never `Send`/`Deliver` (which
  now panic on overflow under the `CanSend`/`CanDeliver` convention) and never
  `RetrieveIncoming`/`RetrieveOutgoing`. Restoring directly also fires no hooks
  and notifies no connection.
- A restored full outgoing buffer is consistent: on resume the owner's
  `CanSend` check sees it full and retries, exactly as before the checkpoint.
- Preserve FIFO order and validate `Src`/`Dst` names against rebuilt ports.
- Leave hooks and connection/component pointers untouched; setup rebuilt them.

### 8. Event registry and serial engine snapshot

- Add a `timing` event codec registry for `timing.Event` values.
- Add built-in codecs for `modeling.TickEvent` and
  `modeling.TimerFiredEvent`. Custom event producers register their event types.
- Change live `SerialEngine` ordering to deterministic same-time ordering. The
  cleanest path is to store an engine-local sequence number with each scheduled
  event and order by `(time, sequence)` inside primary and secondary queues.
- Save current time, primary queue, secondary queue, and next schedule sequence.
- Restore queues without invoking handlers.
- Validate each restored event's `HandlerID` against the rebuilt handler registry.
- Reject `ParallelEngine` checkpoints.

### 9. Tick scheduler and wakeup state

- Add snapshot/restore methods for `modeling.TickScheduler` private fields:
  handler ID, frequency, secondary mode, next tick time, and scheduled flag.
- Ensure restored scheduler state matches restored queued tick events.
- Remove any blanket reset behavior for full checkpoints; reset is only valid for
  partial or phase-boundary restart modes.

### 10. ID generator entity

- Save generator kind and next counter.
- Restore before loading entities that may validate message/event IDs.
- Reject parallel ID generator checkpoints unless deterministic semantics are
  added.

### 11. Shared resources

- Implement `mem.Storage` checkpointing as binary or binary-plus-metadata:
  capacity, unit size, and allocated units. Sort unit addresses before writing.
- Add validation for zero unit size and capacity bounds.
- Implement `vm.PageTable` checkpointing with an explicit JSON DTO sorted by
  `(PID, VAddr)`, then rebuild internal maps/lists on load.
- Add resource-level spec/shape compatibility checks so rebuilt resources cannot
  silently accept incompatible payloads.

### 12. Connections and network state

- Implement checkpoint methods for registered persistent connections.
- `noc/directconnection` can serialize its embedded component state, especially
  `State.NextPortID`, while treating the plug-in port list as rebuilt topology.
- Add DTOs for switching endpoints/switches when runtime state is not already in
  their component `State`.
- Ensure every persistent connection has exactly one archive entry and is not
  also saved as a separate component unless that is its registered runtime owner.

### 13. Load validation and post-load hooks

- Add a load context (extending the `LoadCheckpoint` signature) with lookup by
  entity name, port name, handler ID, message ID, and resource name.
- Validate unresolved names/IDs as strict errors.
- Run post-load hooks only after all raw state is loaded.
- Use post-load hooks for consistency checks and derived caches, not for normal
  scheduling.

### 14. Tests

Unit tests:

- Entity-path escaping, sorted archive output, unknown-entry rejection, and
  coverage checks (missing and extra entities).
- Build ID success and mismatch failure.
- Generic component state round trip and spec mismatch failure.
- Event-driven component pending wakeup round trip.
- Buffer and pipeline JSON round trips.
- Message and event codec success, unknown type failure, and unsupported field
  failure.
- Port buffer round trips with multiple message types.
- Serial engine event queue round trips, including same-time ordering and
  secondary events.
- Tick scheduler snapshot consistency.
- ID generator round trip.
- Storage and page-table resource round trips.
- DirectConnection round-robin cursor round trip.

Integration tests:

- Keep a simple phase-boundary save/load test as a smoke test.
- Checkpoint with non-empty port incoming and outgoing buffers.
- Checkpoint with pending tick events and event-driven timer events.
- Checkpoint with same-time primary and secondary events.
- Checkpoint in the middle of a memory transaction and compare uninterrupted vs
  resumed execution.
- Checkpoint a memory hierarchy with registered storage and page table resources.

Negative tests:

- Save while using `ParallelEngine`.
- Load into missing or extra entities.
- Load with mismatched topology, spec hash, resource shape, build ID, message
  type, or event type.
- Load with corrupted archive payloads.

## Milestones

Phase B progressed non-linearly — the foundation, the polymorphic codecs, and the
queue machinery landed early to de-risk — so the original linear list no longer
maps cleanly. This section records what is done and defines the milestones that
remain.

### Done

- **Foundation**: the archive format (a build-id entry plus per-entity payloads,
  no manifest), build identity, simulation `SaveCheckpoint`/`LoadCheckpoint`
  orchestration, and saved-vs-rebuilt coverage validation.
- **Quiescent serializers**: generic components (`State` + spec hash), the ID
  generator, and `mem.Storage`, with strict spec/shape compatibility checks.
- **Queues and polymorphism**: `queueing.Buffer`/`Pipeline` JSON, the
  `messaging.Msg` and `timing.Event` codec registries, and non-empty port
  buffers serialized through the message registry.
- **Serial engine (partial)**: deterministic same-time event ordering by
  `(time, sequence)`, and the serial-engine event-queue snapshot.
- **Resume oracle (partial)**: a mid-transaction checkpoint/resume for a
  port-less component (a pending tick in the queue) matches the uninterrupted
  run.
- **Scheduler and wakeup consistency (Milestone A)**: after restore, each
  component reconciles its scheduler/wakeup guard with the restored queue —
  `timing.SerialEngine.NextEventTimeForHandler` reports the earliest scheduled
  event for a handler, and `simulation.LoadCheckpoint` runs a post-load pass
  calling `AfterCheckpointLoad`. `Component` derives its `TickScheduler` guard;
  `EventDrivenComponent` (now itself checkpointable) derives `pendingWakeup` the
  same way. Two regression tests assert no duplicate/missed tick or wakeup.
- **Page table serializer**: `vm.PageTable` saves its shape (log2 page size)
  plus, per process and sorted by PID, the pages in list order; load validates
  the shape and rebuilds the per-process tables. The last quiescent resource.
- **Mid-transaction resume oracle (Milestone B gate)**: a deterministic driver
  and an ideal memory controller over a direct connection. `RunUntil` (a new
  deterministic engine boundary) stops the source run with requests in flight;
  a fresh sim loads the checkpoint and runs to completion with the same
  reads-verified count, no data mismatch, and the same end time as an
  uninterrupted reference. This exercises non-empty port buffers, the
  controller's in-flight transactions, pending primary/secondary ticks, the
  connection's round-robin cursor, and the Milestone A guard derivation
  together — retiring the connection/NoC "implicitly covered" item for a real
  assembly. Tracing is off (its task-ID side-table consumes the global ID
  generator).

### Remaining

Drive B's stretch; C runs alongside.

**B (stretch). Hierarchy resume oracle** (§11, §12, §14)
- Extend the resume oracle to a cache + TLB + page-table hierarchy, validating
  the `vm.PageTable` serializer in a live assembly and surfacing any hidden
  (non-`State`) runtime in the cache/TLB components. Optional for the gate,
  which the ideal-controller oracle already meets, but raises confidence for
  production assemblies.

**C. Hardening and ship** (breadth; §13, §14)
- Negative tests: corrupted/truncated archive, spec-hash / topology /
  resource-shape / build-id mismatch, and unknown codec tags (fill gaps; several
  already pass).
- A standalone connection/NoC round-trip test (DirectConnection round-robin
  cursor; a one-switch network checkpoint).
- A determinism regression: a fixed simulation checkpointed at several boundaries
  all resume to identical final state.
- Refresh the docs and examples, and declare Phase B complete.

## Open Questions

- Should `simulation` expose a read-only public entity inventory, or should
  checkpointing remain an internal `Simulation` capability?
- Should message/event codec keys always be `%T`, or should packages be able to
  opt into explicit stable names for readability?
- How strict should post-load pending-work validation be for components that
  intentionally leave messages buffered without a scheduled tick?
- Is build identity based only on VCS/build info enough, or should it also include
  a hash of registered checkpoint codec keys?

(The earlier question about non-nil `Info` / per-component relinking by message
ID is resolved: messages are now immutable value snapshots that carry no
task-ID fields, so a buffered message round-trips completely and needs no
relinking.)

## Recommendation

Start Phase B with the foundation and quiescent-state milestones, but design every
interface around non-quiescent restore from day one. The hard part is not JSON;
the hard part is making event queues, port buffers, scheduler guards, connection
state, and shared resources agree about the same future. Keep the event queue
authoritative, make all unsupported cases fail loudly, and use the
uninterrupted-vs-resumed tests as the merge gate for full support.
