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
ports. The tracing ownership change below updates the port checkpoint encoding.

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

## Tracing task ownership

This is a breaking pre-v5 change (related to #503). Migrate downstream callers,
including MGPUSim, in this order:

1. Add `uint64` task-ID fields with JSON tags to each transaction in component
   `State`, alongside the request metadata. Keep distinct fields for the
   receive task and every independently outstanding send task. Coalesced
   requests each retain their own receive ID. These fields must pass
   `ValidateState` and participate in checkpoints.
2. Store the return of `TraceReqReceive(domain, msg) uint64` on admission.
   Replace every `MsgIDAtReceiver(msg, domain)` with that stored ID.
3. Store `TraceReqInitiate(domain, msg, parentID) uint64` before `port.Send`.
   Pass the receive ID as `parentID` when forwarding an upstream request.
   Never use the outgoing message ID as a task ID.
4. Change completion to `TraceReqComplete(domain, recvTaskID)` and
   `TraceReqFinalize(domain, sendTaskID)`. Change all resets to
   `EndTaskOnReset(domain, taskID)` for each owned task, before clearing state.
   Delete all forget calls. Zero denotes an unstarted task and is ignored by
   completion, reset, tag, and milestone helpers.
5. Replace incoming-buffer lookups with `port.PeekIncomingTaskID() uint64`,
   read before `RetrieveIncoming`. The port stores IDs in buffer entries and
   saves/restores them with the typed messages. Custom `messaging.Port`
   implementations need this method and should emit `PortBufferTrace` as
   hook `Detail` (message `Item` is unchanged). Recreate old checkpoints.
6. Custom tracers that need parent links use a per-tracer
   `RequestTaskParents`: pass every start through
   `ResolveStart(TaskStart) TaskStart`, then call `End(taskID uint64)` on every
   end. Serialize these calls with the tracer's lock. Do this before filtering.

Removed functions: `MsgIDAtReceiver`, `ForgetMsgIDAtReceiver`,
`MsgIDAtIncomingBuffer`, `ForgetMsgIDAtIncomingBuffer`,
`MsgIDAtOutgoingBuffer`, `ForgetMsgIDAtOutgoingBuffer`, and `EndReqInOnReset`.
`TaskStart` adds `MsgID uint64`; `messaging.Msg` is unchanged. Domain arguments
come first throughout the tracing API.

`DBTracer` and `BackTraceTracer` resolve req_in parents from observed req_out
starts and discard correlation on req_out end. Unmatched requests have no
parent, including requests already in flight at restore. Correlation is
observer state and is not checkpointed. Buffer tasks use the same helper,
including response buffers linked by the message's `RspTo`. Existing task and
parent columns suffice: neither the DBTracer schema nor Daisen changed.

The TLB and MMU-cache control middlewares drop milestones that previously
referred to nonexistent req_in tasks. Other control tracing, such as the
writeback cache's actual flush task, retains its complete task lifecycle.

For example, idealmemcontroller now initializes
`tx.RecvTaskID = tracing.TraceReqReceive(comp, req)` and completes with
`tracing.TraceReqComplete(comp, tx.RecvTaskID)`. Previously it obtained the ID
from `MsgIDAtReceiver(req, comp)` and had to forget the registry entry. The
stored ID now survives a checkpoint without any tracing-global state.

NoC transport tasks follow the same identity rule. `packetization.Flit` adds
`FlitTaskID uint64` alongside the existing `MsgTaskID`; both travel through
checkpointed endpoint/port/switch state. Endpoints end the original IDs at the
receiving side, and switches parent their local tasks to `FlitTaskID`.
`RequestTaskParents` resolves `msg_e2e` parents using the handled message ID
(or `RspTo` for responses), just like buffer tasks. No field is added to
`messaging.Msg`.
