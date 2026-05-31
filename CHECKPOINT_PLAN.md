# Checkpoint Redesign Plan

## Summary

Akita checkpointing should evolve from a quiescent component-state dump into an
explicit simulator runtime snapshot. Setup code keeps rebuilding the object
graph; a checkpoint only restores the runtime facts that can change future
execution.

Design goals:

- Serialize all runtime state that affects future simulated behavior; never
  serialize observers or setup choices (tracers, hooks, monitors, visualizers,
  output handles).
- Let each owner serialize itself. Do not auto-expose internal fields just to
  satisfy JSON. (A separate, deliberate `GetStateByName` access backdoor exists
  for cross-entity access; see Unified Entity Model.)
- Treat every registered runtime object uniformly as an entity. The simulation
  core is type-agnostic for storage and serialization; type-specific behavior
  lives at the registration boundary and in optional capability interfaces.
- Make checkpoint completeness auditable and strict by default.
- Scope the first non-quiescent implementation to `SerialEngine`.
- Do not target long-term backward compatibility. A checkpoint is consumed by
  the same executable that produced it; compatibility is a build-identity
  equality check, not a version-migration system.

## Current State

`simulation.Save` today writes:

- `metadata.json`: engine time and global ID generator state.
- `components/<component>.json`: each component's generic `Spec` and `State`.
- `resources/<resource>.bin`: shared-state payloads (currently `mem.Storage`).

This works for phase-boundary checkpoints when the simulation is quiescent but
is not a transparent snapshot of the full simulator process.

Already done (commit `1e5003d`, "Add checkpoint manifest scaffolding"):

- Map-presence duplicate-name checks for components, ports, connections, and
  resources (replacing the `index != 0` sentinel that let the first name be
  duplicated).
- A `manifest.json` inventory plus a common `Entity` view, connection and
  resource registration, and minimal local `Component`/`Port`/`Connection`
  interfaces. Missing/extra manifest entries are validated, not silently skipped.

Still open:

- Port buffers must be empty; their contents are not saved.
- The event queue is not saved.
- Tick scheduler pending state is not saved.
- Event-driven wakeup guards are reset after load instead of restored.
- Runtime fields outside component `State` are not saved.
- Interface values (`messaging.Msg`, `timing.Event`) do not round-trip through
  generic JSON without type metadata.
- Component `Spec` is restored from the checkpoint instead of being treated as a
  compatibility contract against rebuilt setup.
- Some shared-state resources are binary and may be too large for JSON as the
  default payload format.

## Setup vs Runtime Boundary

Rebuilt from setup code:

- Component construction; port construction and attachment; connection topology.
- Handler registration.
- Tracers, hooks, monitors, visualizers, data recorders, output files.
- Build-time parameters and high-level configuration.

Restored from checkpoint:

- Engine current time, queued events, and same-time ordering metadata.
- Global ID generator state and generator kind.
- Component runtime state.
- Port incoming and outgoing buffers.
- Connection runtime state (round-robin cursors, in-flight transfer state).
- Shared program resources (memory contents, page tables, allocation metadata,
  and other non-timing state shared across components).
- Tick scheduler pending state and event-driven pending wakeup state.

Not restored by default:

- Observability state.
- Random generator state, unless a runtime owner explicitly declares RNG
  continuity as part of its checkpoint contract.

Rule of thumb: if changing a value can change future simulated behavior, it is
runtime state and belongs in the checkpoint, unless the design explicitly makes
it a user-provided continuation input such as a restart RNG seed.

## Unified Entity Model

The simulation acts as a global state manager. Every registered runtime object
is stored uniformly as an entity, and the core never branches on
component/port/connection/resource for storage or serialization. Type
distinctions live only at the edges:

- **Typed registration is the boundary.** `RegisterComponent`,
  `RegisterConnection`, and `RegisterResource` may do type-specific work (attach
  tracing, register with the monitor, pull in a component's ports and resources,
  dedup resources by identity), but each lands the object in one entity store.
- **Type-specific runtime behavior lives in optional capability interfaces**
  that callers assert for, not in the registry. The existing
  `tracing.NamedHookable`, `ResourceOwner`, and `WakeupResetter` checks are
  already this pattern; quiescence checks, port ownership, and post-load wiring
  should follow the same "ask the entity what it can do" idiom.

The simulation package defines minimal local interfaces so it need not import
`messaging`:

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

Concrete `messaging` values satisfy these structurally. Because Go slice return
types are not covariant, components returning `Ports() []messaging.Port` are
adapted at registration time rather than forcing a `messaging` import.

An entity has a name and knows how to serialize itself. The uniform contract is
self-serialization (`Checkpointable`, see Serialization Interfaces), not a plain
`State()` data accessor — so interface values and large binary payloads work,
and owners never expose internals just to satisfy JSON. `Kind`/`Type` survive as
metadata for the manifest, grouping, and debugging, but do not drive registry
logic. Each entity serializes to its own archive entry in its own format; the
save loop is flat. The load path is not flat (see Save and Load Safety).

This entity layer is a reference layer, not a replacement for the concrete
component, port, connection, or resource APIs: the simulation keeps typed
registries for lookups that need concrete methods, while the entity inventory
gives validation, manifest generation, and debugging one vocabulary.

### Global State Access Backdoor

The simulation exposes a deliberate `GetStateByName(name)` backdoor for
cross-entity access, analogous to Unity's `GetComponent`/`Find`. This is a
wanted feature: a "magic" component such as a magic address translator can reach
the globally registered page table or memory directly. Breaking encapsulation
here is an accepted trade-off; the programmer takes the risk.

Guidance:

- Resolve the handle once at setup and cache it. Do not call `GetStateByName` on
  a hot path; it is a name lookup, not a free pointer dereference (this mirrors
  why Unity discourages `Find` in `Update`).
- The natural targets are designed shared state — exactly the `Resource` concept
  (page tables, memory, allocators). Reaching arbitrary component internals by
  name is allowed but is action-at-a-distance; prefer resources for shared data.
- Magic access bypasses the timing model by design. It is safe under
  `SerialEngine`; under a future parallel engine it is a data race and ordering
  hazard, and must be revisited there.
- Keep `GetStateByName` (live access) and `SaveCheckpoint` (serialization) as
  separate contracts even when they point at the same data, so the on-disk
  format is not coupled to the public access API.

### Naming and the Runtime Registry

Checkpoint entries are keyed by stable, user-controlled names. Generated
topologies use dot-delimited hierarchical tokens with bracketed indices:

```text
GPU[1]
GPU[1].SA[1]
GPU[1].SA[1].CU[2]
GPU[1].SA[1].CU[2].MemoryPort
```

Port names extend the owning component name with another token. Names must be
unique across the simulation registry, and name generation must be deterministic
(no map-iteration order), so the same topology rebuilds to the same names.

Persistent runtime owners and shared-state resources are registered globally. A
connection is created and plugged in by setup code, but if it affects runtime
behavior it must be registered as a persistent runtime owner and appear in the
manifest. Shared state (memory, page tables) is registered as a resource, with
components holding references rather than embedding payloads. Rebuilt setup
objects, observability objects, and purely derived wiring stay out of these
registries.

Strict load fails when:

- A saved persistent entry has no rebuilt runtime owner.
- A rebuilt persistent runtime owner has no saved entry.
- A checkpoint contains unknown extra entries.
- A runtime owner or resource name is duplicated.

An opt-in override for extra entries may be added later, but strict is the
default.

## Component and Resource Model

This section captures the v5 component-authoring model and resource-ownership
decisions. They are breaking changes (acceptable for v5) and are the next
implementation milestone after the global state manager.

### A component is Spec + State + Resources

Every component is `modeling.Component[Spec, State, Resources]` (and likewise
`EventDrivenComponent[S, T, R]`). The three parts map exactly onto the runtime
categories:

| Part | Meaning | Serialized? | Rebuilt by |
|------|---------|-------------|------------|
| `Spec` | immutable configuration | as spec/compat hash | setup |
| `State` | mutable runtime data | **yes** — the serializable unit (`StateRef`) | — |
| `Resources` | typed references to shared resources | **no** — wiring | setup |

`modeling.None` (an exported zero-size `struct{}`) is the sentinel for components
with no resources: `Component[Spec, State, modeling.None]`. The third type
parameter is **mandatory and uniform** — every component visibly declares all
three categories rather than a terse two-parameter default. This was chosen over
a generic-alias shortcut so the model is the same everywhere a user (or
downstream) writes a component.

The typed `Resources` slot **supersedes per-field checkpoint tags** for
resources: a reference structurally lives in `Resources`, not in `State`, so no
`checkpoint:"resource"` annotation is needed (see Field Classification).

### No hand-written component struct

A user defines only: `Spec`, `State`, and `Resources` data structs plus their
`Middleware` structs (where the behavior lives). The component *is*
`modeling.Component[Spec, State, Resources]` — there is no per-component wrapper
struct and no accessor methods.

- Middlewares hold a single `*modeling.Component[Spec, State, Resources]` and
  reach everything through it: `comp.Spec`, `comp.State`, `comp.Resources`
  (`comp.Resources.Storage`), `comp.GetPortByName(...)`. The reference fields
  previously duplicated onto middlewares (e.g. a separate `storage`) go away.
- Former accessors (`GetStorage()`, `StorageName()`) become field access
  (`comp.Resources.Storage`, `comp.Spec.StorageRef`).
- The residual a user writes is: three data structs + middleware structs + a thin
  builder/setup call through a generic `NewBuilder[S, T, R]`.

Open item: ports are *not* part of Spec/State/Resources/Middlewares but a
component still needs ports created and attached. Decide whether ports become a
declared part of the component or remain a builder/setup concern.

### The simulation owns shared resources

Shared resources (memory contents, page tables, allocator state) are owned by the
simulation — the global state manager — not by any component. They are top-level
entities, peers of components, reachable by name through `GetStateByName`. A
component never embeds a resource payload in its `State`; it holds only a
reference.

- The durable identifier is the resource's **name** (kept in `Spec`); the
  `Resources` field holds the cached pointer for fast access. The pointer is
  wiring, rebuilt by setup; the resource's contents are restored into the one
  canonical resource object.
- Setup constructs the resource, gives it **one canonical name**, registers it
  once with `RegisterResource`, and injects the reference into the components
  that use it.

### `ResourceOwner` is dropped

The `ResourceOwner` / `component.Resources()` auto-registration is removed. It
made each holder name and register the resource, so a single shared object could
end up registered under multiple names (`A.Storage`, `B.Storage`). Registration
becomes explicit and setup-owned under a canonical name.

- The identity-based dedup in `registerResource` is removed; the global
  name-uniqueness check in `registerEntity` already detects duplicates.
- `Resource` slims toward just `Entity` (drop `Identity()`, and `Kind()` if it is
  dead). A resource is "an entity that happens to be shared state," distinguished
  only by living in the resource registry for enumeration.

### Implementation order

1. `modeling`: add the `R` parameter, the `Resources` field, and `None` to
   `Component` and `EventDrivenComponent` and their builders
   (`NewBuilder[S, T, R]`, `NewEventDrivenBuilder[S, T, R]`).
2. Sweep the ~92 instantiation sites across `mem`/`noc`/`examples`/tests to add
   `, modeling.None`.
3. Convert `idealmemcontroller`/`simplebankedmemory`/`dram` to a `Resources`
   struct, update their middlewares to read `comp.Resources`, drop
   `ResourceOwner` and the wrapper accessors, and slim `Resource` /
   `registerResource`.

## Checkpoint Format

The checkpoint is a single `tar.gz` archive. Consistent with the type-agnostic
core, every entity is one entry; there are no per-kind sections. The logical
entry layout (not loose files on disk) is:

```text
checkpoint/
  manifest.json
  entities/<encoded-entity-name>.json
  entities/<encoded-entity-name>.bin
```

Raw simulation names are not used directly as entry names; use a stable escaping
or hashing scheme that is reversible where practical and immune to path
separators.

The manifest is a flat, auditable inventory of entities; load validates it
before mutating any runtime state. There are no typed buckets — `Kind` on each
entry already distinguishes entity types, and adding a new kind does not change
the manifest struct.

```go
type CheckpointManifest struct {
    Version   int                      `json:"version"`
    CreatedBy string                   `json:"created_by"`
    BuildID   string                   `json:"build_id"`
    Entities  map[string]ManifestEntry `json:"entities"` // keyed by stable entity name
}

type ManifestEntry struct {
    Kind       string `json:"kind"`   // component | port | connection | resource | engine | id-generator
    Path       string `json:"path"`   // encoded archive entry name
    Format     string `json:"format"`
    SpecHash   string `json:"spec_hash,omitempty"`   // components only
    ContentSHA string `json:"content_sha,omitempty"`
}

type Entity struct {
    Kind EntityKind // component | port | connection | resource | engine | id-generator
    Name string
    Type string     // optional type metadata, e.g. "mem.Storage" for resources
}
```

The engine and the ID generator are entities like any other: singletons with
reserved names (e.g. `"engine"`, `"id-generator"`) and their own kinds. Neither
gets a special top-level manifest field, and the ID generator is its own entity
rather than being folded into the engine — the point of the refactor is to have
no special cases. Validation requires exactly one entity of each singleton kind.

Because the inventory is one flat map keyed by name, entity names must be
**globally unique across all kinds**, including the reserved singleton names.
This is already a goal (names unique across the simulation registry); the flat
manifest makes it a hard requirement. The map key is the raw stable name; `Path`
is the encoded archive entry.

`BuildID` is a build-identity fingerprint of the producing executable. Because
checkpoints are not meant to outlive that executable, load requires `BuildID` to
match the current binary and fails loudly otherwise. This replaces version
migrations with a single equality check: a mismatched checkpoint fails fast
instead of silently corrupting restored state. `Version` gates only the manifest
format, not content migration.

## Serialization Interfaces

Serialization is explicit and owned by each runtime state owner.

```go
type Checkpointable interface {
    CheckpointName() string
    CheckpointKind() string
    SaveCheckpoint(ctx SaveContext, w io.Writer) error
    LoadCheckpoint(ctx LoadContext, r io.Reader) error
}

type AfterCheckpointLoad interface {
    AfterCheckpointLoad(ctx LoadContext) error
}
```

`AfterCheckpointLoad` restores derived wiring that cannot be stored directly,
resets guards, and schedules required wakeups — after all raw state is loaded.
Raw load functions must not call behavioral APIs (`Send`, `Deliver`,
`NotifyRecv`, `NotifyPortFree`); those can trigger new events while the restored
graph is incomplete.

Generic component `Spec`/`State` JSON remains as a compatibility layer, with
changed load semantics (see Spec Compatibility). Long term, each component can
define a runtime checkpoint DTO that includes unexported details without making
them public API.

### Field Classification

Components must not carry unclassified mutable fields outside `Spec`, `State`,
ports, and declared runtime resources. Wrapper components and middleware may hold
handles to ports, shared resources, processors, or derived wiring, but every
such field declares a checkpoint policy:

- `checkpoint:"port"` — rebuilt by setup; buffers checkpointed by the port owner.
- `checkpoint:"resource"` — handle to a registered shared resource; checkpointed
  once by the resource registry.
- `checkpoint:"derived"` — rebuilt by `AfterCheckpointLoad`.
- `checkpoint:"observer"` — tracing, monitoring, recorders; never affects
  simulation behavior.
- `checkpoint:"external"` — continuation input supplied by the user at restart,
  such as an RNG seed.

Untagged extra fields fail validation. Validation inspects both wrapper component
structs and middleware structs, since shared-resource handles may live in
middleware.

Note: under the v5 component model (see Component and Resource Model), resource
references live in the typed `Resources` slot rather than as
`checkpoint:"resource"`-tagged fields, so that policy is subsumed by structural
classification — `Spec` is config, `State` is serialized, `Resources` is wiring.

### Spec Compatibility

Setup rebuilds the simulator; a checkpoint must not blindly replace rebuilt setup
with saved setup.

- Save component specs (or canonical spec hashes) and component kind for
  compatibility checking.
- On load, compare rebuilt spec against checkpoint spec, and restore only runtime
  state.
- Fail with a clear error on mismatch. There is no migration fallback (see
  no-backward-compatibility goal).

The spec-hash check is the fine-grained counterpart to the coarse `BuildID`
check: `BuildID` rejects a different executable; the spec hash catches topology
or configuration drift within the same executable.

## Interface Values: Message and Event Registries

Ports contain `messaging.Msg` values; engine queues contain `timing.Event`
values. Because Go has no sum types and cannot reconstruct a concrete type from
JSON through an interface, polymorphic decode requires type tags plus a
constructor registry. This is inherent to Go, not a design preference.

### When a registry is needed

A constructor registry is needed for exactly one situation: an interface-typed
value that **setup does not rebuild**, so it must be reconstructed from
checkpoint data alone.

| Kind | Rebuilt by setup? | Needs constructor registry? |
|------|-------------------|------------------------------|
| Message in a port buffer | no (transient) | **Yes** |
| Event in the engine queue | no (transient) | **Yes** |
| Component, Port, Connection, Resource | yes | No |
| Event handler reference | yes (it is a component) | No |

Setup-rebuilt objects already exist at load time: load looks the instance up by
name and calls `LoadCheckpoint` to fill it in place; the instance knows its own
type. For them, the entity name-to-instance map *is* the registry, and handlers
are resolved by string ID against that map. Only messages and events have no
pre-existing instance, so only they need a constructor registry. This is a direct
consequence of the setup-vs-runtime split — rebuilding from setup confines
registries to two transient interface types instead of one per component type.

### Registries live low; the core stays decoupled

The simulation core never imports `messaging` or `mem` and never touches a
`messaging.Msg`. Each interface-value owner serializes itself and uses a registry
that lives in its own package:

- The **message-codec registry lives in `messaging`**. `mem`, `mem/vm`,
  `mem/datamover`, `noc/packetization`, and user packages already import
  `messaging`, so they register without an import cycle. The `Port` (in
  `messaging`) uses it to serialize its own buffers.
- The **event-codec registry lives in `timing`**. The engine (in `timing`) uses
  it to serialize its own queues. Built-in tick/timer/secondary events are
  registered by the framework; custom events register here too.

The core only orchestrates opaque `Checkpointable` entities that hand it bytes,
so it stays type-agnostic and free of concrete `messaging`/`timing` imports.

### Registration mechanism

```go
type TypedPayload struct {
    Type    string          `json:"type"`
    Payload json.RawMessage `json:"payload"`
}

// in messaging
func RegisterMsg(construct func() Msg) { /* key derived from %T of construct() */ }

// in mem
func init() { messaging.RegisterMsg(func() messaging.Msg { return &ReadReq{} }) }
```

- Common case: one line per message type; the default codec is
  `json.Marshal`/`Unmarshal`. Custom codecs only for types with non-serializable
  fields.
- `init()` registration gives a coverage guarantee: if a `*mem.ReadReq` can
  appear at runtime, then `mem` is linked, so its `init()` ran, so its codec is
  registered. The only failure mode is a package author forgetting to register a
  type they defined — caught by the unknown-type error at first checkpoint.
- Because checkpoints are consumed by the same executable, the registry key can
  be derived from `fmt.Sprintf("%T", ...)`; no hand-maintained stable names, no
  rename drift, no per-payload version.
- Unknown type tags are load errors, never silent drops.
- For scale, a `go:generate` tool can emit registrations for all types embedding
  `messaging.MsgMeta`, reducing the per-type burden to zero.

This registry replaces, rather than adds to, existing complexity: today
`mem/vm/addresstranslator/state.go` hand-writes a `%T` switch that copies message
fields by hand; the default JSON codec subsumes that.

### References are by name/ID, not pointer

Akita already expresses cross-object references as stable names: message
`Src`/`Dst` are `messaging.RemotePort` (a string), and events reference handlers
by `HandlerID` (a string). Serialization stores IDs and re-links by lookup in the
post-load phase; there is no pointer graph to rebuild. The unifying rule:
everything crosses the boundary as a stable name/ID, never as a pointer.

The only violator is raw `interface{}` fields such as `mem.ReadReq.Info` /
`mem.WriteReq.Info` (`json:"-"`), which hold transient back-references to
sender-owned state:

- Initial: a codec rejects checkpointing when such a field is non-nil and the
  type defines no explicit encoding. Fail loud; never drop.
- Later: treat `Info` as derived wiring re-linked in the post-load phase by
  matching message ID against the sender's checkpointed inflight table.
  Per-component work, not a global registry concern.

### Writer burden

- Messages: many, user-defined → the one-line registration (or codegen) is the
  only recurring tax, and only for non-quiescent checkpoints.
- Events: few, mostly framework-provided → low burden.
- Components, ports, connections, resources: zero registry, zero per-type
  registration; they self-serialize in place.

## Per-Entity Checkpointing

### Ports

Ports hold `messaging.Msg` values and have send/receive side effects, so they
use a raw DTO:

```go
type PortCheckpoint struct {
    Name             string         `json:"name"`
    IncomingCapacity int            `json:"incoming_capacity"`
    OutgoingCapacity int            `json:"outgoing_capacity"`
    Incoming         []TypedPayload `json:"incoming"`
    Outgoing         []TypedPayload `json:"outgoing"`
}
```

Save snapshots buffer contents without popping. Load validates capacities against
the rebuilt port, recreates buffers directly, preserves FIFO order, and defers
all notifications to the post-load phase. The post-load phase schedules the same
future work that was possible before checkpointing (incoming messages wake the
owning component; outgoing messages wake the connection) — see Load Ordering for
how this avoids double-scheduling.

### Connections

Topology is rebuilt by setup; connection runtime state is restored. Example:
`DirectConnection` has a round-robin cursor (`NextPortID`) that affects delivery
order. Switching-network endpoints and switches may have buffers/pipelines that
are already component state but still need manifest coverage. Avoid saving a
connection both as a component and as a separate runtime owner: each persistent
runtime owner has exactly one manifest entry.

### Engine

The engine checkpoint includes current time, the primary and secondary event
queues, and same-time ordering metadata. Initial support is `SerialEngine` only;
parallel engines are rejected.

Event queues currently order only by time, so deterministic replay needs a
tie-breaker. Options:

- Expose event IDs from `EventBase` via an optional interface and restore by
  `(time, secondary, event_id)`.
- Add an engine-local insertion sequence number and checkpoint it.

Whichever is chosen, the **live engine must use the same deterministic ordering**
so resume matches uninterrupted execution. The engine provides explicit
snapshot/restore APIs rather than exposing queue internals, and load validates
that every restored event has a registered handler.

### Tick and Wakeup State

Scheduled work must not be silently lost. For `modeling.TickScheduler`, the
checkpoint needs handler ID, frequency, secondary-event mode, whether a tick is
scheduled, and the scheduled tick time. For `modeling.EventDrivenComponent`,
resetting the pending wakeup guard is not enough once the event queue is
restored; the pending wakeup is restored or reconstructed from restored
`TimerFiredEvent` entries. See Load Ordering for the single-source-of-truth rule.

### Shared Resources

Shared resources are non-timing program state referenced by multiple components.
`mem.Storage` is one kind; the model also covers `vm.PageTable`, process
metadata, allocator state, and loaded program metadata. The simulation package
knows only the generic contract and does not import concrete resource packages;
concrete packages adapt to it.

The resource interface is the slim, access-side contract (serialization methods
were removed with Phase B; each resource type defines its own serializer when
Phase B is rebuilt — see below):

```go
type Resource interface {
    Entity         // Name() string
    Kind() string  // candidate for removal; slims toward just Entity
    Identity() string
}
```

The simulation keeps a global registry of resources and **owns** them; components
hold references (see Component and Resource Model). `ResourceOwner` and the
auto-registration through `RegisterComponent` are **dropped** — setup constructs
each shared resource, names it canonically, and registers it once with
`RegisterResource`. The saved unit is the shared resource, not the component
field pointing to it.

- `mem.Storage` stays binary (contents may be too large for JSON; the binary
  format already captures capacity, unit size, and allocated units). Sort storage
  map keys before writing for byte-for-byte reproducibility.
- `vm.PageTable` uses an explicit JSON DTO, sorted by `(PID, VAddr)` for
  deterministic files; rebuild internal maps/lists from the DTO on load.

  ```go
  type PageTableCheckpoint struct {
      Log2PageSize uint64    `json:"log2_page_size"`
      Pages        []vm.Page `json:"pages"`
  }
  ```

Resource metadata (name, kind, payload path, format, kind-specific fields such as
capacity/unit size, optional content hash) is JSON in the manifest. Compression
and chunking can be reconsidered after the format stabilizes.

## Save and Load Safety

### Save preconditions

Transparent non-quiescent checkpointing needs an atomic view of the simulator.
The initial design requires:

- `SerialEngine` only.
- Save only while the engine is stopped or paused outside an event handler.
- No concurrent component, port, connection, or resource mutation during save.
- A defined lock ordering for snapshot collection to avoid deadlock.

### Atomicity and packaging

The checkpoint is one `tar.gz` archive. A large simulation can have tens of
thousands of entities (ports especially), so loose per-entity files create real
inode/IO pressure; write entity payloads directly into the archive stream rather
than materializing a directory and taring it afterward. Write
`checkpoint.tar.gz.tmp` and rename on success for atomicity. Incompressible
binary payloads (memory dumps) may be stored uncompressed to save CPU.

### Load ordering

Save is a flat loop; load is staged because entities have inter-dependencies:

1. Validate the manifest and the `BuildID` equality check before mutating any
   runtime state. A checkpoint from a different executable fails fast.
2. Restore all raw state with no behavioral calls.
3. Re-link references by name/ID (`Src`/`Dst`, `HandlerID`, `Info`) against the
   entity map; an unresolved ID is a strict error.
4. Run `AfterCheckpointLoad` for derived wiring, guards, and wakeups.

**Single source of truth for pending work.** The restored event queue,
scheduler/wakeup guards, and port post-load notifications all describe future
work and must not overlap. The restored event queue is authoritative;
scheduler/wakeup guards are reconstructed to match it rather than re-scheduling,
and port post-load notifications only fire for work the queue does not already
represent. Nothing is double-scheduled or dropped.

Parallel-engine checkpointing is rejected until deterministic queue capture,
worker state, and concurrent mutation semantics are defined.

## Migration Phases

The work splits into two phases with very different risk profiles. Phase A is a
structural refactor whose oracle is "the simulation still runs bit-identically."
Phase B is serialization, whose oracle is "resume equals uninterrupted
execution." Phase A also ships value on its own (state inspection, monitoring,
the `GetStateByName` backdoor), which makes the split low-regret.

Why non-quiescent is required, not optional: Akita is event-driven, so an empty
event queue means the simulation is complete — there is no mid-run moment with an
empty queue. Today's quiescent checkpoint only works because the driver
re-injects the next kernel's events; that fails for a single multi-day kernel,
whose pending ticks and timers *are* the simulation. Serializing the event queue
is mandatory for any resume that is not "start the next kernel."

### Phase A — Global State Manager (no persistence)

Make the entire mutable runtime state enumerable, named, classified, and
reachable, without serializing anything yet.

- Collapse the typed registries and the entity view into one entity inventory;
  make the engine and the ID generator entities too (each a singleton with a
  reserved name).
- Define the `Entity` contract and `GetStateByName`. Reserve the
  `SaveCheckpoint`/`LoadCheckpoint` method slots now (stubbed) so Phase B does not
  force a contract rework; keep `GetStateByName` distinct from serialization.
- Classify every component and middleware field with a checkpoint policy and add
  field-shape validation. Use the exact tags Phase B consumes.
- Move shared state (page tables, memory) into registered resources where it is
  not already.
- Lock in deterministic naming plus a regression test.
- Add encoded archive entry names and add `BuildID` to the manifest (enforced on
  load in Phase B).

**Exit criterion:** every piece of mutable runtime state can be enumerated,
named, classified, and reached; the simulation runs identically to before the
refactor; nothing is serialized.

### Phase B — Serialization

Add persistence to the entities the state manager already exposes.

1. **Component and shared-resource serialization** — implement the reserved
   `SaveCheckpoint`/`LoadCheckpoint` slots; stop overwriting rebuilt specs
   (compare spec/spec hash); keep `mem.Storage` binary; add `vm.PageTable` as a
   JSON resource; enforce the `BuildID` check before mutating state.
2. **Engine snapshot for `SerialEngine`** — save/restore time and both queues;
   add deterministic same-time ordering (applied to the live engine too); add the
   `timing` event-codec registry with built-in codecs; reject parallel engines.
3. **Message registry and port buffers** — add the `messaging` message-codec
   registry (`init()` registration, default JSON codec); save/restore port
   buffers without side effects; reject non-nil non-serializable fields such as
   `Info`.
4. **Scheduler and wakeup state** — serialize or reconstruct `TickScheduler` and
   event-driven pending wakeups; remove the blanket `ResetWakeup` for full
   snapshots.
5. **Connection runtime state** — register persistent connections; add DTOs;
   exactly one manifest entry per runtime owner.
6. **Load ordering** — implement the staged load and the single-source-of-truth
   rule for pending work.
7. **Full non-quiescent resume** — enforce save preconditions; compare
   uninterrupted vs checkpoint/resume execution; treat missing/extra/unsupported
   entries as errors.
8. **Packaging and coverage** — `tar.gz` archive streaming with `.tmp`+rename;
   more built-in codecs (consider `go:generate`); store incompressible payloads
   uncompressed.
9. **Deferred: parallel engine** — define deterministic queue capture and
   worker-state semantics first.

## Testing Plan

Unit tests:

- Name-registry duplicate detection (including the first registered entry) and
  deterministic name generation across repeated rebuilds.
- `GetStateByName` resolution: hit, miss (clear error), and handle validity
  across save/load (load mutates in place).
- `BuildID` equality: matching build loads, mismatched build fails fast.
- Archive round trip (pack/unpack `tar.gz`).
- Manifest read/write, path encoding, strict validation; missing, extra,
  duplicate, and unsupported entries.
- Component DTO round trips; spec compatibility success and mismatch failure.
- Shared-resource metadata, storage binary payload, and page-table JSON round
  trips.
- Component and middleware field-shape validation.
- Port buffer round trips with multiple concrete message types.
- Message/event codec rejection for unknown types and unsupported
  non-serializable fields.
- Event queue round trips (primary, secondary, same-time, tick, timer, custom).
- Tick scheduler and event-driven wakeup restoration.
- Connection runtime state round trips.

Integration tests:

- Keep the current quiescent save/load determinism test.
- Non-quiescent checkpoint with messages in port buffers.
- Pending ticks survive load; pending event-driven wakeups survive load.
- Direct-connection round-robin cursor survives load.
- Memory-hierarchy test comparing uninterrupted vs checkpoint/resume execution.

Negative tests:

- Missing component/port/connection/engine/id-generator/resource entry.
- Unknown message type; unknown event type.
- Rebuilt topology missing a saved port/component/connection.
- Spec mismatch; `BuildID` mismatch.
- Non-quiescent save on an unsupported parallel engine.
- Checkpoint while a message has unsupported non-serializable fields.

## Open Questions

- Should strict load allow an opt-in mode that ignores extra checkpoint entries?
- Which runtime owners may declare external continuation inputs (e.g. RNG seed)
  instead of checkpointing internal state? (Note: the default of not restoring
  RNG state is in tension with the runtime-state rule of thumb; revisit.)
- What exactly constitutes `BuildID`, and how is it computed deterministically?
- Should the post-load `Info` re-link (per-component, by message ID) ship in the
  initial non-quiescent milestone, or stay deferred behind reject-if-non-nil?
- In the v5 component model, are ports a declared part of a component (alongside
  Spec/State/Resources/Middlewares) or do they remain a builder/setup concern?

## Resolved Decisions

- No cross-version migration; checkpoints are consumed by the same executable,
  enforced by a `BuildID` equality check that fails loudly on mismatch.
- The artifact is a single `tar.gz`, not a directory of loose files.
- The simulation is a global state manager: one uniform entity store, typed
  registration at the boundary, and a deliberate `GetStateByName` backdoor;
  breaking encapsulation through it is accepted.
- Polymorphic decode uses constructor registries only for transient interface
  values setup does not rebuild: messages (registry in `messaging`) and events
  (registry in `timing`). Setup-rebuilt objects self-load in place. Registration
  is `init()`-based with a default JSON codec and `%T`-derived keys; unknown type
  tags are hard errors.
- The work proceeds as Phase A (global state manager) then Phase B
  (serialization).
- Phase A (global state manager) is implemented; Phase B serialization code was
  removed so the codebase represents only Phase A. Phase B will be rebuilt fresh.
- v5 component model: a component is `Component[Spec, State, Resources]` with a
  mandatory third type parameter (`modeling.None` when unused). The simulation
  owns shared resources; components hold references; `ResourceOwner` is dropped.
  Users define Spec/State/Resources/Middlewares with no per-component wrapper
  struct. (See Component and Resource Model.)

## Recommendation

Do Phase A first as a self-contained milestone: a structural refactor with a
simple oracle (the simulation still runs identically) that ships value on its own
through state inspection and the `GetStateByName` backdoor. Its product is a
runtime whose entire mutable state is enumerable, named, and classified.

Then do Phase B, treating event queues, port buffers, scheduler state, connection
state, and load ordering as one coordinated non-quiescent milestone, since they
depend on each other for correct resume. Shape Phase A's entity contract around
Phase B's known requirements so the split stays clean rather than speculative.
