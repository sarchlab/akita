# Checkpoint Redesign Plan

## Summary

Akita checkpointing should evolve from a quiescent component-state dump into an
explicit simulator runtime snapshot. Simulation setup should continue to be
rebuilt by user code, while checkpoint files restore runtime facts that can
change future execution.

The design goal is:

- Do not serialize observers or setup choices such as tracers, hooks,
  monitoring servers, visualizers, or output file handles.
- Do serialize all runtime state that affects future simulated behavior.
- Do not expose internal fields only to make JSON serialization work.
- Make checkpoint completeness auditable and strict by default.
- Keep the first non-quiescent implementation scoped to `SerialEngine`.

## Current Model

Today `simulation.Save` writes a checkpoint directory with:

- `metadata.json`: engine time and global ID generator state.
- `components/<component>.json`: each component's generic `Spec` and `State`.
- `resources/<resource>.bin`: shared-state payloads, currently used for
  `mem.Storage` through a package-local adapter.

This is effective for phase-boundary checkpoints when the simulation is
quiescent. It is not a transparent snapshot of the full simulator process.

Current limitations:

- Port buffers must be empty.
- Event queues are not saved.
- Connections and topology are rebuilt from setup code.
- The simulation has no connection registry, so connection runtime state cannot
  be inventoried globally.
- Tick scheduler pending state is not saved.
- Event-driven component wakeup guards are reset after load instead of restored.
- Runtime fields outside component `State` are not saved.
- Interface values, such as `messaging.Msg` and `timing.Event`, do not
  round-trip through generic JSON without explicit type metadata.
- Component `Spec` is restored from checkpoint instead of being treated as a
  compatibility contract with rebuilt setup.
- Missing component and resource checkpoint files are skipped silently during
  load.
- Component and port duplicate-name checks use `0` as a sentinel, so the first
  registered name can currently be duplicated.
- Some shared-state resources are binary, not JSON, and may be large enough
  that JSON should not be the default payload format.

## Target Model

Introduce a global checkpoint root that owns the runtime state tree. Setup code
still builds the object graph, registers components, creates ports, attaches
connections, and configures tracing or monitoring. Loading a checkpoint then
fills that rebuilt graph with runtime state.

Conceptual checkpoint layout:

```text
checkpoint/
  manifest.json
  engine.json
  globals.json
  components/
    <encoded-component-name>.json
  ports/
    <encoded-port-name>.json
  connections/
    <encoded-connection-name>.json
  resources/
    <encoded-resource-name>.json
    <encoded-resource-name>.bin
```

Raw simulation names should not be used directly as filenames. Use an escaping
or hashing scheme that is stable, reversible where practical, and immune to
path separator issues.

Conceptual manifest:

```go
type CheckpointManifest struct {
    Version       int                         `json:"version"`
    CreatedBy     string                      `json:"created_by"`
    Engine        ManifestEntry               `json:"engine"`
    Globals       ManifestEntry               `json:"globals"`
    Components    map[string]ManifestEntry    `json:"components"`
    Ports         map[string]ManifestEntry    `json:"ports"`
    Connections   map[string]ManifestEntry    `json:"connections"`
    Resources     map[string]ManifestEntry    `json:"resources"`
}

type ManifestEntry struct {
    Kind       string `json:"kind"`
    Path       string `json:"path"`
    Format     string `json:"format"`
    Version    int    `json:"version"`
    SpecHash   string `json:"spec_hash,omitempty"`
    ContentSHA string `json:"content_sha,omitempty"`
}

```

The manifest is the auditable inventory of everything expected in the
checkpoint. Loading should validate the manifest before mutating runtime state.

Runtime registries should also expose a common entity view:

```go
type EntityKind string

const (
    EntityKindComponent  EntityKind = "component"
    EntityKindPort       EntityKind = "port"
    EntityKindConnection EntityKind = "connection"
    EntityKindResource   EntityKind = "resource"
)

type Entity struct {
    Kind EntityKind
    Name string
    Type string
}
```

This entity registry is a reference layer, not a replacement for the concrete
component, port, connection, or resource APIs. The simulation should store
entities as the common runtime inventory while retaining typed registries for
lookups that need concrete APIs. It gives checkpoint validation, manifest
generation, and debugging tools one vocabulary for registered runtime objects.
The `Type` field is optional type metadata within the entity kind; for resources
it is the resource kind, such as `mem.Storage`.

## Package Boundary Interfaces

The simulation package should define the minimal local interfaces that its
registry needs, rather than importing concrete messaging abstractions:

```go
type Component interface {
    Name() string
}

type Port interface {
    Name() string
    NumIncoming() int
    NumOutgoing() int
}

type Connection interface {
    Name() string
}
```

Concrete `messaging.Component`, `messaging.Port`, and `messaging.Connection`
values satisfy these contracts structurally. Because Go slice return types are
not covariant, existing components with `Ports() []messaging.Port` should be
adapted at registration time instead of forcing the simulation package to
import `messaging`.

## Stable Names and Runtime Registry

Checkpoint entries should be keyed by stable, user-controlled names. Generated
topologies should use dot-delimited hierarchical tokens with bracketed indices,
for example:

```text
GPU[1]
GPU[1].SA[1]
GPU[1].SA[1].CU[2]
GPU[1].SA[1].CU[2].MemoryPort
```

Port names extend the owning component name with another dot-delimited token.
Names must be unique across the simulation registry. Before this plan depends
on names, fix registry duplicate checks so they use map presence checks instead
of `index != 0`.

The simulation should also maintain registries for persistent runtime owners
and shared-state resources. A connection can still be created and plugged in by
setup code, but if it affects runtime behavior it must be registered as a
persistent runtime owner and included in the manifest. Shared state, such as
memory contents or page tables, should be registered globally with the
simulation. Components should keep references to these resources rather than
embedding the resource payload in component state. Rebuilt setup objects,
observability objects, and purely derived wiring should remain outside these
registries.

Strict load should fail when:

- A saved persistent entry has no rebuilt runtime owner.
- A rebuilt persistent runtime owner has no saved entry.
- A checkpoint contains unknown extra entries.
- A runtime owner or resource name is duplicated.

An override mode for extra entries can be added later, but strict mode should be
the default.

## Save and Load Safety

Transparent non-quiescent checkpointing needs an atomic view of the simulator.
The initial design should require:

- `SerialEngine` only.
- Checkpoint save only while the engine is stopped or paused outside an event
  handler.
- No concurrent component, port, connection, or shared-resource mutation during
  save.
- A defined lock ordering for snapshot collection to avoid deadlock.
- A temporary checkpoint directory plus final rename, so partially written
  checkpoints are not mistaken for valid checkpoints.

Parallel engine checkpointing should be rejected until deterministic queue
capture, worker state, and concurrent mutation semantics are defined.

## Setup vs Runtime Boundary

Rebuilt from setup code:

- Component construction.
- Port construction and attachment.
- Connection topology.
- Handler registration.
- Tracers, hooks, monitors, visualizers, data recorders, and output files.
- Build-time parameters and high-level configuration.

Restored from checkpoint:

- Engine current time and queued events.
- Event ordering metadata needed for deterministic replay.
- Global ID generator state and generator kind.
- Component runtime state.
- Port incoming and outgoing buffers.
- Connection runtime state, such as round-robin cursors or in-flight transfer
  state.
- Shared program resources, such as memory contents, page tables, allocation
  metadata, and other non-timing state shared across components.
- Tick scheduler pending state.
- Event-driven component pending wakeup state.

Not restored by default:

- Observability state.
- Random generator state, unless a runtime owner explicitly declares that RNG
  continuity is part of its checkpoint contract.

Rule of thumb: if changing a value can change future simulated behavior, it is
runtime state and belongs in the checkpoint unless the design explicitly makes
that value a user-provided continuation input, such as a restart RNG seed.

## Serialization Interfaces

Checkpoint serialization should be explicit and owned by each runtime state
owner. Do not expose internal fields simply because JSON needs to see them.

Proposed owner interface:

```go
type Checkpointable interface {
    CheckpointName() string
    CheckpointKind() string
    SaveCheckpoint(ctx SaveContext, w io.Writer) error
    LoadCheckpoint(ctx LoadContext, r io.Reader) error
}
```

Proposed post-load hook:

```go
type AfterCheckpointLoad interface {
    AfterCheckpointLoad(ctx LoadContext) error
}
```

`AfterCheckpointLoad` should restore derived runtime wiring that cannot be
stored directly, reset guards, and schedule required wakeups after all raw
state is loaded. Raw load functions should not call behavioral APIs such as
`Send`, `Deliver`, `NotifyRecv`, or `NotifyPortFree`; those APIs can trigger
new events while the restored graph is incomplete.

Generic component `Spec` and `State` JSON can remain as a compatibility layer,
but its load semantics should change:

- Save both the spec or spec hash and the runtime state.
- On load, compare saved spec/spec hash with the rebuilt spec.
- Restore only runtime state by default.
- Fail on spec mismatch unless a migration or explicit override is requested.

Long term, each component should define a runtime checkpoint DTO. That DTO can
include unexported implementation details without making them public API.

## Component Field Shape

Components should not carry unclassified mutable fields outside `Spec`,
`State`, ports, and explicitly declared runtime resources. Wrapper components
and middleware may still need handles to ports, shared resources, processors,
or derived wiring, but every such field should have a declared checkpoint
policy.

Recommended field policies:

- `checkpoint:"port"`: rebuilt by setup; port buffers are checkpointed by the
  port owner.
- `checkpoint:"resource"`: handle to a registered shared resource, such as
  `mem.Storage` or `vm.PageTable`; checkpointed exactly once by the resource
  registry.
- `checkpoint:"derived"`: rebuilt by `AfterCheckpointLoad`.
- `checkpoint:"observer"`: tracing, monitoring, progress bars, data recorders,
  or other observability state; never affects simulation behavior.
- `checkpoint:"external"`: continuation input supplied by the user at restart,
  such as an RNG seed when deterministic RNG continuity is intentionally not
  checkpointed.

Untagged extra fields should fail validation. This validation should inspect
both wrapper component structs and middleware structs, because important shared
resource handles may live in middleware today.

## Port Checkpointing

Ports contain `messaging.Msg` interface values and have side effects on normal
send and receive operations. Port checkpointing should use a raw DTO:

```go
type PortCheckpoint struct {
    Name             string         `json:"name"`
    IncomingCapacity int            `json:"incoming_capacity"`
    OutgoingCapacity int            `json:"outgoing_capacity"`
    Incoming         []TypedPayload `json:"incoming"`
    Outgoing         []TypedPayload `json:"outgoing"`
}
```

Save should snapshot buffer contents without popping messages. Load should:

- Validate capacities against the rebuilt port.
- Recreate incoming and outgoing buffers directly.
- Preserve FIFO order.
- Defer all notifications until the post-load phase.

The post-load phase should schedule the same future work that would have been
possible before checkpointing. For example, a port with restored incoming
messages should wake its owning component, and a port with restored outgoing
messages should wake its connection.

## Connection Checkpointing

Connection topology is rebuilt by setup code. Connection runtime state is
restored from checkpoint.

Examples:

- `DirectConnection` has a round-robin cursor (`NextPortID`) that affects
  delivery order.
- Switching-network endpoints and switches may have buffers or pipelines that
  are already component state, but they still need manifest coverage.

The design should avoid saving both a connection as a component and the same
connection as a separate runtime owner. Each persistent runtime owner should
have exactly one manifest entry.

## Engine Checkpointing

The engine checkpoint should include:

- Current simulation time.
- Primary event queue.
- Secondary event queue.
- Event ordering metadata for same-time events.

The initial implementation can support `SerialEngine` only.

Event queues currently order events only by time. Deterministic replay needs a
tie-breaker. Options:

- Expose event IDs from `EventBase` through a new optional interface and use
  `(time, secondary, event_id)` as the restore order.
- Add an engine-local insertion sequence number and checkpoint that sequence.

The engine should provide explicit snapshot/restore APIs rather than exposing
internal queue fields. Load should validate that every restored event has a
registered handler in the rebuilt setup.

## Tick and Wakeup State

Tick scheduler state should be serialized directly or reconstructed from the
restored event queue with equivalent future behavior. Silent loss of scheduled
work should not be allowed.

For `modeling.TickScheduler`, the checkpoint needs enough information to
restore:

- Handler ID.
- Frequency.
- Secondary-event mode.
- Whether a tick is scheduled.
- The scheduled tick time.

For `modeling.EventDrivenComponent`, resetting the pending wakeup guard after
load is not enough once event queues are restored. The pending wakeup state
should be restored or reconstructed from restored `TimerFiredEvent` entries.

## Message and Event Registries

Full checkpointing requires serializing interface values.

Ports contain `messaging.Msg` values. Engine queues contain `timing.Event`
values. These need type tags and registered codecs.

Conceptual envelope:

```go
type TypedPayload struct {
    Type    string          `json:"type"`
    Version int             `json:"version"`
    Payload json.RawMessage `json:"payload"`
}
```

Message registry responsibilities:

- Map stable type names to Go message constructors or custom codecs.
- Marshal a `messaging.Msg` into a typed envelope.
- Unmarshal a typed envelope back into a concrete message.
- Register core message types from `mem`, `mem/vm`, `mem/datamover`,
  `noc/packetization`, and examples/tests as needed.

Event registry responsibilities:

- Support built-in tick events.
- Support event-driven timer events.
- Support secondary events.
- Allow custom event types to register checkpoint codecs.

Unknown type tags should be load errors, not silent drops.

Some current message fields are intentionally non-serializable. For example,
`mem.ReadReq.Info` and `mem.WriteReq.Info` are `json:"-"`. A registry codec
should reject checkpointing when such fields are non-nil unless the message type
defines an explicit way to encode them.

## Shared Resource Checkpointing

Shared resources represent non-timing program state that can be referenced by
multiple components. `mem.Storage` is one resource kind, but the same model
should cover `vm.PageTable`, process metadata, allocator state, loaded program
metadata, and other semantic state that is not owned by a single component
pipeline.

The simulation package should only know this generic shared-state contract. It
should not import concrete resource packages such as `mem`; concrete packages
adapt their resources to the interface.

Proposed resource interfaces:

```go
type Resource interface {
    Name() string
    Kind() string
    Format() string
    FileExtension() string
    Identity() string
    Save(w io.Writer) error
    Load(r io.Reader) error
}

type ResourceOwner interface {
    Resources() []Resource
}
```

The simulation should keep a global registry of `Resource` entries.
Components may expose the resources they reference so `RegisterComponent` can
register them automatically, but the saved unit is still the shared resource,
not the component field that points to it.

`mem.Storage` resources should keep binary contents for the initial design.
Memory contents may be too large for JSON, and the current binary storage
format already captures capacity, unit size, and allocated storage units.

`vm.PageTable` resources should use an explicit JSON DTO, for example:

```go
type PageTableCheckpoint struct {
    Log2PageSize uint64    `json:"log2_page_size"`
    Pages        []vm.Page `json:"pages"`
}
```

Sort page-table entries by `(PID, VAddr)` during save for deterministic files.
On load, rebuild the internal maps and lists from the DTO.

Use JSON for resource metadata:

- Resource name.
- Resource kind.
- Payload path.
- Format version.
- Kind-specific metadata, such as storage capacity and unit size.
- Optional content hash.

If byte-for-byte reproducible checkpoint files matter, sort storage map keys
before writing the binary payload. Compression and chunking can be added after
the format stabilizes.

## Spec Compatibility

Setup should rebuild the simulator. A checkpoint should not blindly replace
rebuilt setup with saved setup.

Recommended behavior:

- Save component specs or canonical spec hashes for compatibility checking.
- Include component kind/version when available.
- On load, compare rebuilt spec against checkpoint spec.
- Fail with a clear error on mismatch unless an explicit migration or override
  is requested.

This keeps checkpointing focused on runtime state while still detecting
accidental topology or configuration drift.

## Migration Phases

1. Fix name-registry correctness and add strict manifest scaffolding.
   - Fix duplicate-name checks for components and ports.
   - Add connection/runtime-owner registration.
   - Add generic shared-state resource registration without depending on `mem`.
   - Add encoded checkpoint paths.
   - Add manifest read/write and strict validation while preserving current
     quiescent behavior.

2. Refactor component and shared-resource compatibility behavior.
   - Introduce explicit checkpoint interfaces.
   - Adapt `modeling.Component` and `modeling.EventDrivenComponent` as default
     implementations.
   - Stop blindly overwriting rebuilt specs on load.
   - Keep `mem.Storage` binary, with resource metadata in the manifest.
   - Add `vm.PageTable` as a JSON resource.
   - Add component and middleware field-shape validation.

3. Add engine snapshot APIs for `SerialEngine`.
   - Save and restore current time, primary queue, and secondary queue.
   - Add deterministic same-time ordering metadata.
   - Add built-in codecs for tick and timer events.
   - Reject parallel engines explicitly.

4. Add message registry and raw port-buffer checkpointing.
   - Add built-in codecs for core message types.
   - Save and restore incoming/outgoing port buffers without side effects.
   - Add post-load notifications to schedule required work.

5. Restore scheduler and wakeup guards.
   - Serialize or reconstruct `TickScheduler` pending state.
   - Serialize or reconstruct event-driven pending wakeups.
   - Remove the current blanket `ResetWakeup` behavior for full snapshots.

6. Add connection runtime checkpointing.
   - Register persistent connections.
   - Add DTOs for connection-owned runtime state.
   - Ensure each runtime owner appears exactly once in the manifest.

7. Add full non-quiescent `SerialEngine` checkpoint/resume.
   - Enforce save safety preconditions.
   - Compare uninterrupted execution with checkpoint/resume execution.
   - Treat missing, extra, or unsupported runtime entries as errors.

8. Expand coverage and migration support.
   - Add more built-in message and event codecs.
   - Add format-version migration hooks only after version 1 stabilizes.
- Reconsider JSON vs binary chunks for very large shared resources.

9. Defer parallel engine checkpointing.
   - Define deterministic queue capture and worker-state semantics first.

## Testing Plan

Unit tests:

- Name-registry duplicate detection, including the first registered entry.
- Manifest read/write, path encoding, version handling, and strict validation.
- Missing, extra, duplicate, and unsupported manifest entries.
- Component checkpoint DTO round trips.
- Spec compatibility success and mismatch failure.
- Shared-resource metadata, storage binary payload, and page-table JSON round
  trips.
- Component and middleware field-shape validation.
- Port buffer round trips with multiple concrete message types.
- Message codec rejection for unknown types and unsupported non-serializable
  fields.
- Event queue round trips with primary, secondary, same-time, tick, timer, and
  custom events.
- Tick scheduler and event-driven wakeup guard restoration.
- Connection runtime state round trips.

Integration tests:

- Continue current quiescent save/load determinism test.
- Add a non-quiescent checkpoint test with messages in port buffers.
- Add a scheduled-event checkpoint test where pending ticks survive load.
- Add an event-driven component test where pending wakeups survive load.
- Add a direct-connection arbitration test where the round-robin cursor survives
  load.
- Add a memory hierarchy test comparing uninterrupted execution with
  checkpoint/resume execution.

Negative tests:

- Missing component, port, connection, engine, globals, or resource entry.
- Unknown message type.
- Unknown event type.
- Rebuilt topology missing a saved port, component, or connection.
- Spec mismatch.
- Attempted non-quiescent save on unsupported parallel engine.
- Attempted checkpoint while a message has unsupported non-serializable fields.

## Open Questions

- Should version migrations be built into the loader now, or added after the
  first manifest version stabilizes?
- Should strict load allow an opt-in mode that ignores extra checkpoint entries?
- Which runtime owners should be allowed to declare external continuation inputs,
  such as RNG seed, instead of checkpointing their internal state?
- How should custom users register message and event codecs without importing
  every optional Akita package?

## Initial Recommendation

Start with the hardening work: fix name uniqueness, add a connection/runtime
owner registry, add a strict manifest, and preserve current quiescent behavior.
That creates a reliable inventory of the state tree. Then move to event queues,
port buffers, scheduler state, and connection state as one coordinated
non-quiescent milestone, because those pieces depend on each other for correct
resume behavior.
