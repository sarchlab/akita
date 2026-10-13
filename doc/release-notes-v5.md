# V5 release notes

## Simulation package migration

Runtime packages are now under `sim/`; `simulation`, `monitoring2`, and
`daisen2` are renamed to `sim`, `monitoring`, and `daisen`.

**Live monitoring is now opt-in.** Downstream simulators such as MGPUSim must
supply `WithMonitor(monitoring.NewMonitor())` to retain their monitoring server.
`Build()` binds and starts the supplied monitor, and `Terminate()` stops it.
Omitting `WithMonitor` leaves runs headless while tracing remains independent.
The `WithoutMonitoring` and `WithMonitorPort` options are removed.

Live progress bars move from `daisen.ProgressBar` to `monitoring.ProgressBar`.
The memory access agent accepts a small `ProgressTracker` interface through
`SetProgressTrackers`, keeping UI types out of component code.

See the [migration guide](./tutorial/migration.md) for updated imports,
monitor wiring, and port configuration. MGPUSim migration is separate.

## Switching-network payload delivery

Switching networks preserve the complete original message, including its typed
payload. Only flit 0 carries that message; body flits contain the routing and
reassembly header. Checkpoints retain one copy of the message across flit queues
and reassembly state. `packetization.AssembledMsg` and the `Delivery` role are
removed. Existing checkpoints with the old network layout must be recreated.
Link bandwidth and latency modeling are a separate change.

## Port and connection packages

Buffered ports move to `sim/messaging/twowaybuffered` as `Port`, constructed by
`NewPort(in, out)`. Ideal connections move from `noc/directconnection` to
`sim/messaging/direct` as `Connection`, constructed with
`NewConnection(name, simulation, frequency)`. Frequency is explicit; ports and
connections have no component `Definition`, default spec, or builder. Old paths
and constructors are removed.

NoC builders now explicitly support buffered ports and ideal direct connections.
The port factory is removed; endpoint device wiring rejects unsupported device
ports. Port checkpoints store messages without tracing IDs.

## Explicit component assembly

Component builders accept Spec and Resources, without `WithPorts`. Build leaves
runtime State and Middlewares unset. Use `component.BindPort("Top", port)` to
assign the slot, owner and full name, and `connection.BindPort(port)` to attach
both sides of a link. Ports use `NewPort(in, out)`; `SetOwner`, `SetConnection`,
and connection `PlugIn` are removed in favor of one-time bindings.

Call `simulation.Initialize()` after wiring and before seeding work, running,
or saving/loading checkpoints. Validation failures leave setup editable; once
initializers begin, topology is frozen. Downstream simulators must migrate this
call order. Domain configuration and builders remain a separate change.

## Spec checkpoint integrity

Before: Spec fields could use `json:"-"` or be unexported; after: every Spec
field must be exported and encoded. `json:",omitzero"` is also rejected on
fields with custom `IsZero() bool` methods, which could hide distinct values.
Move rebuilt data to State or Resources.
Custom JSON must supply both methods and round-trip the configured value.

## Ticking frequency validation

Before: zero `Spec.Freq` failed at the first tick; after: Build rejects it.
Ticking Specs must declare `Freq timing.Freq` in runtime, inspector, and
`modelingtest.CheckTicking`; supply a nonzero value before Build.

## Simulation checkpoint build identity

Before: `s.SaveCheckpoint(path, id)` / `s.LoadCheckpoint(path, id)`; after:
`s.SaveCheckpoint(path)` / `s.LoadCheckpoint(path)`. Build identity is derived
automatically from the running binary's build information, stored in the
checkpoint, and checked on load. There is no public override.
Per-entity checkpoint methods are unchanged.

## Simulation-scoped task tracking

This pre-v5 change moves component tracing commands and task types to
`sim/tasks`. Enable `WithTaskTracking` (also implied by monitor and trace-on-start),
and subscribe observers through `s.Tasks().Subscribe(observer, owners...)`
before running. With tracking disabled, `s.VisTracer()` is nil and no tracing
tables are created. `WithTaskTracking` alone creates and subscribes the tracer;
call `s.VisTracer().StartTracing()` to begin recording. Components receive the
service through `TaskTracker()`; tracking is nil when disabled. Built-in observers
move from `sim/tracing` to
`sim/tasks/tracing` (the package name remains `tracing`); update those imports.
Task types and their implementation live directly in `sim/tasks`, with private
state and helpers. Framework assembly uses the exported `NewTracker`,
`Tracker.RegisterPort`, `Tracker.FreezeSubscriptions`, `Tracker.SaveCheckpoint`,
and `Tracker.LoadCheckpoint` APIs. Ordinary applications use the simulation
builder; checkpoint entity names and optional archive membership stay in `sim`.

- Replace `StartTask` with `id := tasks.Start(owner, tasks.StartInfo{...})`.
  It allocates an independent task ID; do not pass a caller-generated ID.
- Use `StartReqOut` before `port.Send`, and `StartReqIn` on admission. Store each
  returned handle in State. End with `EndReqOut`, `EndReqIn`, or
  `End(owner, EndInfo{ID: id})`. Reset paths use the same end commands;
  `EndOnReset` is removed because it had no distinct behavior.
- Use `tasks.Tag`, `tasks.Milestone`, `AddTag`, and `AddMilestone`. Replace buffer
  task lookups with `AddBufferMilestone(owner, port, msg, milestone)` before
  retrieval. It drops events for untracked messages without misattribution.
- Replace custom Tracer methods with `TaskStarted(Task)`, `TaskEnded(Task)`,
  `TagAdded(Tag)`, and `MilestoneAdded(Milestone)`, optionally embedding
  `tasks.NopObserver`. Both lifecycle notifications carry the basic task record;
  the end includes the original start time. Observers no longer resolve parents.
- Remove `CollectTrace`, the buffer collector helpers, task hook positions,
  `RequestTaskParents`, `PeekIncomingTaskID`, and `PortBufferTrace`. Custom ports
  report generic `PortBufferChange` head facts in message-hook Detail. Hooks run
  under the port lock and must not call buffer operations; Owner/Name are safe.

Request helper migration (end commands take the stored task ID):

| Previous helper | New helper |
|---|---|
| `TraceReqReceive` | `StartReqIn` |
| `TraceReqComplete` / `CompleteReq` | `EndReqIn` |
| `TraceReqInitiate` | `StartReqOut` |
| `TraceReqFinalize` / `FinalizeReq` | `EndReqOut` |

`messaging.Msg` is unchanged. Request/NoC handles remain in component or flit
State, and the Tracker owns records and buffer identities. DRAM subtransaction
IDs remain simulation identities with separate tracing handles. Port checkpoint
entries are messages again, without the temporary tracing version/ID fields.
Pre-v5 checkpoint formats should be recreated after upgrading.

Tracker open records, owner names, buffer identities, and request-parent
correlation are checkpointed. Restored tasks retain IDs, parents, and original
start times; `Detail` and observer histories are omitted. Rebuild fresh observers.
An off-to-on restore starts with an empty tracker and ignores unknown handles;
an on-to-off restore ignores the optional tracker payload. All other entity
coverage checks remain strict.

Recording started mid-run drops earlier tags and milestones of tasks already
in flight. When such a task ends during recording, its original parent and full
interval are recorded without replaying its history. Open-task snapshots remain
deferred. Daisen's stored task/parent schema is unchanged, and its frontend ID
types are numeric. MGPUSim migration follows this Akita change separately.

See [tasks](../sim/tasks/README.md) and [observers](../sim/tasks/tracing/README.md).
