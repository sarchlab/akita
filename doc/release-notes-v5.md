# V5 release notes

## Direct configuration for ideal networks, caches, and TLBs

NoC construction now uses one generic switch-creation and device-connection
method with concrete configurations. Unsupported non-ideal link settings and
bandwidth/version presets are removed. Flit size and input/output widths remain
separate direct settings. Internal queues and network-owned ports derive their
capacities from the corresponding widths; caller-owned device ports keep their
sizes. Forwarding now honors multi-flit input/output budgets with rotating
priority. PCIe/NVLink default port depths change from 16 to 1 at width one.
NVLink uses fewest-hop routing. These changes can alter contention and timing;
custom setup routing remains public.

Caches use explicit set counts and directory/bank pipeline widths. Cache request
queues use request width; directory/bank output queues use their pipeline widths.
TLB output depth equals lookup width. Both caches and the TLB use `NumSets`,
`NumWays`, and `NumMSHREntries`. Architectural MSHR and write-buffer capacities
remain public; internal queue capacities are not configuration fields. The
writeback bank retains its reserved upward lane; its default directory output
queue grows from one to two entries. DRAM calculations and presets are unchanged.
See the migration guide; these pre-v5 Spec/topology changes require rebuilding
old checkpoints.

## CLI shell and retired legacy implementations

The `akita` executable, installation path, and help remain available.
`akita component --create NAME` and `akita check PATH` now report that their
features are unavailable and exit with status 1, without creating or modifying
files or validating components. Their obsolete implementations and templates
are removed, and `akita/cmd` moves to `akita/internal/cmd`.

These deprecated command spellings preserve recognition only. A replacement
CLI and its command migration policy are planned for [V5.1 (#523)](https://github.com/sarchlab/akita/issues/523).
Use the component tutorials, `modelingtest` checks, and the `inspect` Go package
in the meantime.

## Public-surface cleanup

`Msg.TrafficClass`, `simplebankedmemory.Spec.Capacity`/`BankSelectorKind`,
`idealmemcontroller.Spec.CacheLineSize`, and
`networkconnector.LinkEndSwitchParameter.PortName` are removed. Size the backing
`Storage` explicitly. The generic `DataReader` API, `NewDataRecorderWithDB`, and
the writer's `ListTables` are removed; use SQLite directly for custom analysis.
The unused `mem.AddressConverter` interface and duplicate `mem.ConvertAddress`
helper are removed; the concrete `mem.InterleavingConverter` remains available.

`networkconnector.LinkEndSwitchParameter` and `LinkEndDeviceParameter` rename
`NumInputChannel`/`NumOutputChannel` to `InputWidth`/`OutputWidth`, matching
the NoC builders. These remain positive flits-per-cycle limits with unchanged
queue sizing. Update struct literals and field accesses; no aliases remain.
The topology checkpoint fingerprint changes, so rebuild old NoC checkpoints.

`mmuCache.Resources.LowModulePort` and `UpModulePort` move to `Spec.LowModule`
and `Spec.UpModule` (JSON keys `low_module` and `up_module`). `Resources` is now
empty. These peer addresses participate in checkpoint compatibility; rebuild
old mmuCache checkpoints. Routing behavior is unchanged.

The standalone ticking scheduler, ID generator, source archive helpers, Daisen
server library/assets, monitoring assets, and acceptance/test helpers are now
internal. The Daisen command and component-level scheduling remain supported.
Tick event implementation paths change; rebuild checkpoints from older builds.

`memcontrolprotocol.State` and its constants are removed: components define
private execution states. Zero is now an invalid `Command`, rejected as
unsupported. DRAM's control-state migration preserves its behavior and all its
configuration fields. Progress-bar fields and mutex become private; use
`Snapshot()` for read access and the existing update methods for counters.
The JSON shape is preserved, start time is populated, reads are synchronized,
and completing an already-removed bar is safe. Monitoring no longer registers
HTTP pprof handlers on the default mux merely by being imported.
The standalone DGX acceptance programs no longer start profiling HTTP servers.
The `one_to_one` acceptance program drops its unused checkpoint topology helper;
its message-delivery scenario remains unchanged.

Custom setup routers, public cache/MSHR/LRU helpers, and `mmuCache`/`TransLatency`
remain. MMU-cache correctness fixes and DRAM
configuration work are deferred; non-ideal links are deferred beyond this ideal-only API.

## Engine pause API

`timing.Engine`, `SerialEngine`, and `ParallelEngine` remove `Pause` and
`IsPaused`. The control surface is `RequestPause`, `Continue`, `State`, and
`Inspect`.

- Replace `Pause()` with `RequestPause().Wait(ctx)` and handle the returned error.
  Use a context appropriate to the caller, such as an HTTP request's context.
- Replace the engine's `IsPaused` method with `engine.State().IsPaused()`.
  `EngineState.IsPaused` is true for `EnginePaused` and `EngineResuming`: a
  queued continue has not taken effect yet. It is false for `EnginePausing`.

Handlers and hooks may call `RequestPause`, but must not call `Wait`:
acknowledgment needs the current event or parallel batch and its hooks to finish.
Wait from another goroutine. Cancelling the wait does not cancel the request.

For MGPUSim's `amd/driver/driver.go`, migrate
`Pause(); TickLater(); Continue()` to
`RequestPause().Wait(ctx); TickLater(); Continue()`. Handle the wait error before
calling `TickLater` or `Continue`, and keep this sequence outside handlers and
hooks. Continue also returns an error that must be handled. MGPUSim source
changes follow separately.

## Simulation package migration

Runtime packages are now under `sim/`; `simulation`, `monitoring2`, and
`daisen2` are renamed to `sim`, `monitoring`, and `daisen`.

**Live monitoring is now opt-in.** Downstream simulators such as MGPUSim must
supply `WithMonitor(monitoring.MakeBuilder().Build())` to retain their monitoring server.
Successful `Initialize()` binds and starts the supplied monitor after State
exists, and `Terminate()` stops it before recording closes.
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
`MakeBuilder().WithSimulation(sim).WithSpec(spec).Build()`. The port `Spec`
sets `IncomingCapacity` and `OutgoingCapacity`; `DefaultSpec()` uses 1/1.
`WithSimulation` is required and both capacities must be positive. Ideal connections move from `noc/directconnection` to
`sim/messaging/direct` as `Connection`, constructed with
`NewConnection(name, simulation, frequency)`. Frequency is explicit. Connections retain this constructor. Buffered ports use
their own value builder and are unnamed, unowned, and unregistered until binding.
Old paths and constructors, including `twowaybuffered.NewPort`, are removed.

NoC builders now explicitly support buffered ports and ideal direct connections.
The port factory is removed; endpoint device wiring rejects unsupported device
ports. Port checkpoints retain messages, buffer task IDs, and wait boundaries.

## Explicit component assembly

Component builders accept Spec and Resources, without `WithPorts`. Build leaves
runtime State and Middlewares unset. Use `component.BindPort("Top", port)` to
assign the slot, owner and full name, and `connection.BindPort(port)` to attach
both sides of a link. Ports use the simulation-aware builder; `SetOwner`, `SetConnection`,
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

## Checkpoint and termination lifecycle

Use `s.SaveCheckpoint(path)` and `s.LoadCheckpoint(path)`. Checkpoints contain
entity payloads without a build ID. Inventory, Spec hashes, and entity-specific
validation remain; callers ensure code/state compatibility. Unregistered event
types now fail during save, before archive writing, rather than during restore.

Active runs, including paused runs and direct engine calls, reject saving,
loading, and termination. A complete checkpoint excludes engine startup.
`Terminate() error` cleans up once, retains its result, and permanently rejects
subsequent execution and checkpoint operations. There is no recorder exit hook;
callers close independently constructed recorders themselves.

## Namespaced recording tables

Every table Akita writes into a recording now carries the `akita$` prefix:
`akita$trace`, `akita$milestone`, `akita$tag`, `akita$segments` (formerly
`daisen$segments`), `akita$component_spec`, `akita$port`, `akita$source`,
`akita$exec_info`, and `akita$location`. Column names are unchanged. Tables an
application writes through `s.DataRecorder()` may now use generic names such as
`trace` or `location`; avoid the `akita$` prefix. `CreateTable` panics with a
message naming the table when the recording already has one with that name,
ignoring ASCII case as SQLite does. Update custom SQL to the new names. Daisen
reads only the new names, so recordings from earlier versions do not open in
this Daisen.

## Middleware and construction guards

Use `event.Middleware.Handle(timing.Event)` for event components and
`Handle(timing.Event) bool` for `ticking.Middleware` and `wakeup.Middleware`.
The shared `modeling.Middleware` interface is removed. Middleware fields are
exported for reflection at initialization; the pipeline is cached for dispatch.

Resource builders require `WithSimulation(s)` and register in `Build(name)`.
Handlers reject duplicate registration names. Port capacities must be positive;
`Send` checks a nonzero message ID and a bound connection before any effects.
Unused ports may remain unconnected. Direct connections reject zero frequency
before registration. IDs already start at 1; `RspTo == 0` remains valid.

## Simulation-scoped task tracking

This pre-v5 change moves component tracing commands and task types to
`sim/tasks`. Enable `WithTaskTracking` (also implied by monitor and trace-on-start),
and subscribe observers through `s.TaskTracker().Subscribe(observer, scopes...)`.
Subscribe before seeding work to see starts,
or join mid-run for future events. Scopes accept `naming.Named`; components include
their ports, while port scopes select only that port. All must share the tracker.
With tracking disabled, `s.VisTracer()` is nil and no tracing tables are created.
`WithTaskTracking` alone creates only the in-memory tracker; also use
`WithOutputPath` to create a recorder and visualization tracer for manual
`StartTracing`. Obtain `tracker := s.TaskTracker()`; component models expose the
same service through `TaskTracker()`. `tasks.Owner` embeds only `naming.Named`.
Ports retain the simulation supplied by `WithSimulation` and obtain its tracker
and clock directly. `BindTaskTracker` is removed; custom owners register their
ports after assigning the owner and name. `tasks.NewTracker` remains public.
The tracker owns task operations and stamps time and IDs using simulation
services. Nil tracker methods do nothing and allocate no IDs. Built-in observers
move from `sim/tracing` to
`sim/tasks/tracing` (the package name remains `tracing`); update those imports.
The shared `LeakRecorder` test helper lives in module-root `internal/tracingtest`;
it is not part of the public v5 API.
Task types and their implementation live directly in `sim/tasks`, with private
state and helpers. Framework assembly uses the exported `NewTracker`,
`Tracker.SaveCheckpoint`,
and `Tracker.LoadCheckpoint` APIs. Ordinary applications use the simulation
builder; checkpoint entity names and optional archive membership stay in `sim`.

- Replace `tracing.StartTask` with `id := tracker.StartTask(owner, tasks.StartInfo{...})`.
  It allocates an independent task ID; do not pass a caller-generated ID.
- Use `StartReqOut` before `port.Send`, and `StartReqIn` on admission. Store each
  returned handle in State. End with `EndReqOut`, `EndReqIn`, or
  `tracker.EndTask(id)`. Reset paths use the same end commands;
  `EndOnReset` is removed because it had no distinct behavior.
- Use `tasks.Tag`, `tasks.Milestone`, `AddTag`, and `AddMilestone`. Replace buffer
  task lookups with `port.AddIncomingMilestone(tasks.Reason(kind, what))` at
  resolved wait-stage boundaries. `AddOutgoingMilestone` is symmetric; both
  operate on the current head. Retrieve signatures remain unchanged. Empty,
  untracked, and zero-duration waits are ignored. Retrieval fills only remaining
  time with consumer/owner (incoming) or network_busy/connection (outgoing).
- Replace custom Tracer methods with `TaskStarted(Task)`, `TaskEnded(Task)`,
  `TagAdded(Tag)`, and `MilestoneAdded(Milestone)`, optionally embedding
  `tasks.NopObserver`. Both lifecycle notifications carry the basic task record;
  the end includes the original start time. Observers no longer resolve parents.
- Remove `CollectTrace`, the buffer collector helpers, task hook positions,
  `RequestTaskParents`, `PeekIncomingTaskID`, and `PortBufferTrace`. Custom ports
  implement the two milestone methods and may use `tasks.Owner` to start/end
  their buffer tasks directly, retaining task IDs and wait boundaries privately.
  Generic message hooks carry Msg in Item and nil Detail. Hooks run under the
  port lock and must not call buffer operations; Owner/Name are safe.

Task helper migration (end commands take the stored task ID):

| Previous helper | New helper |
|---|---|
| `tracing.StartTask` / `tasks.Start` | `tracker.StartTask` |
| `tracing.EndTask` / `tasks.End` | `tracker.EndTask` |
| `TraceReqReceive` | `StartReqIn` |
| `TraceReqComplete` / `CompleteReq` | `EndReqIn` |
| `TraceReqInitiate` | `StartReqOut` |
| `TraceReqFinalize` / `FinalizeReq` | `EndReqOut` |

`messaging.Msg` is unchanged. Request/NoC handles remain in component or flit
State, and the Tracker owns records and parent correlation. DRAM subtransaction
IDs remain simulation identities with separate tracing handles. Port checkpoint
entries contain messages, buffer task IDs, and last milestone times.
Pre-v5 checkpoint formats should be recreated after upgrading.

Tracker open records, owner/component scope names, and request-parent
correlation are checkpointed separately from port buffer entries.
Restored tasks retain IDs, parents, and original
start times; `Detail` and observer histories are omitted. Rebuild fresh observers.
An off-to-on restore starts with an empty tracker and ignores unknown handles;
an on-to-off restore ignores the optional tracker payload. All other entity
coverage checks remain strict.

Recording started mid-run drops earlier tags and milestones of tasks already
in flight. When such a task ends during recording, its original parent and full
interval are recorded without replaying its history. Open-task snapshots remain
deferred. Daisen's stored task/parent columns are unchanged, and its frontend ID
types are numeric. Statistical observers count the full interval of each received
end, including time before subscription or restore. BusyTimeTracer uses interval
union and no longer exposes `TerminateAllTasks`.

MGPUSim migration follows this Akita change separately: update task/tracing
imports, retain returned request handles in component State, migrate observer
subscriptions to `TaskTracker()`, and convert buffer-reason calls to port
milestones at stage transitions. Existing no-argument retrieve calls stay as-is;
custom port implementations and mocks need both new milestone methods.

See [tasks](../sim/tasks/README.md) and [observers](../sim/tasks/tracing/README.md).

## Event and accessor naming

The v5 Go API uses the following names. Update downstream callers and interface
implementations; the old names are removed.

- `timing.EventBase.Time_` → `At` and `HandlerID_` → `Handler`.
  `Time()`, `HandlerID()`, and `IsSecondary()` are unchanged.
- `ticking.TickEvent` → `ticking.Event`.
- `ticking.MakeTickEvent(id, handlerID, time)` →
  `ticking.MakeEvent(id, time, handlerID)`, matching `timing.MakeEventBase`.
- `modeling.Scheduler` → `modeling.PendingEventGuard`. Its `Snapshot` and
  `Restore` methods are unchanged; it records whether the component's own tick
  or wakeup is pending and when, so restore does not schedule a duplicate.
- Writeback's internal `pipelineMW.GetSpec()` → `Spec()`.
- Concrete `*vm.PageTable` exposes `Log2PageSize()` instead of
  `GetLog2PageSize()`; both MMU and GMMU validate it.
- `monitoring/internal/static.GetAssets()` and `daisen/internal/static.GetAssets()` → `Assets()`.
- `mshr.Entry.GetPID()` / `GetAddress()` →
  `Matches(pid vm.PID, addr uint64) bool`; cache and TLB entries compare both.
- `memprotocol.AccessReq` and its accessor methods are removed. Switch on
  `ReadReq`/`WriteReq` and use their unchanged `Address` and `PID` fields.

Simulation behavior and the event field keys `time` and `handler_id` are
unchanged. Tick checkpoint type tags now use `tickscheduler.Event`, derived from the
internal Go type; `ticking.Event` remains a public alias. The component checkpoint guard is named `pending_event_guard` instead
of `scheduler`, with `scheduled` and `at` fields. Saved checkpoints from before
this cleanup are not supported; no legacy type-tag alias or schema migration
is provided.

## NoC assembly at Initialize

NoC connect calls now build and bind immediately. Remove `EstablishRoute` and
`EstablishNetwork`: `simulation.Initialize()` computes routes from
completed wiring and returns errors for disconnected or invalid routing. Changing
tables after Initialize is not supported. `BuildSwitches`,
`PortAddress`, `routing.Route`, `Table.Snapshot`, and internal-port return values
are removed. Custom setup routers remain supported. Internal endpoint/switch
routing fields leave Spec; registered topology resources preserve checkpoint
configuration validation. Old checkpoints must be rebuilt. Mesh still supports
rectangular growth, empty transit tiles and repeated AddTile merges. Non-ideal
transport and mutable/adaptive runtime routing are deferred.

### Result-changing NoC changes

Mesh switch arbitration now uses deterministic port binding order. Under
contention, arbitration order and simulated timing can differ from v4 and depend
on assembly order. No public routing hook configures arbitration priority.
Network-owned incoming and outgoing buffers now follow their respective
processing widths, and forwarding supports multiple flits per output per cycle.
The smaller PCIe/NVLink default queues can also change results. `routing.Table` is
a concrete type; custom setup routers remain supported. Checkpoint fingerprints
record computed routes rather than Go router/table type names.

## Final extension contracts (#538)

`vm.PageTable` is now concrete; consumers hold `*vm.PageTable`. The exported
`modeling.Component` interface is removed while concrete model hooks remain.
PortOwner includes identity and `BindPort`; Connection retains `BindPort`, drops
Hookable, and uses `NotifySend(port)`. One Port contract supplies read-only
`IncomingBuf()`/`OutgoingBuf()` views with `Size` and real `Capacity`; nil denotes
an absent buffer. Milestones remain on Port. NoC's concrete port restriction and
wire's deferred status are unchanged.

The exported Monitor contract contains only Start/Stop. Simulation provides copied
Components/Ports inventories, removes its Monitor getter, and starts the monitor
after initialization. Monitor registration methods are removed; buffer inspection
includes State and omits absent buffers. See the migration guide for caller changes.

## Simulation services and lifecycle API

Builders and elements share `modeling.Simulation`, replacing `timing.Simulation`.
It exposes the engine, ID allocation, task tracker, and typed registration for
components, ports, connections, and named resources. `RegisterResource` accepts
`naming.Named`; no resource marker or base type is required. Use `messaging.Port`
directly. `sim.Entity`, `sim.Component`, `sim.Connection`, and `sim.Resource` remain.

Call `s.Initialize()` on the concrete simulation after wiring. Framework setup
validation and component initialization are internal; component authors only
implement optional `State.Initialize(*Comp)` and `Middlewares.Initialize(*Comp)`.
Checkpoint helpers and `timing.Engine.WithIdle` retain their existing APIs.

Framework lifecycle builders require a simulation created by `sim.MakeBuilder`,
or a wrapper embedding `*sim.Simulation` to retain its private lifecycle state.
Forwarding only the exported `modeling.Simulation` methods does not supply that
state and is rejected with a diagnostic during construction.
