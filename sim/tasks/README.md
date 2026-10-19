# Simulation tasks

Enable in-memory tracking explicitly with
`sim.MakeBuilder().WithTaskTracking().Build()`. This creates no recording;
`DataRecorder()` and `VisTracer()` stay nil. To record manually, also supply
`WithOutputPath("trace.sqlite3")`, then call `s.VisTracer().StartTracing()`.
`WithVisTracingOnStart` and `WithMonitor` enable both tracking and recording;
the former starts recording immediately. Tracking is disabled by default.

Tracking, subscribing, and database recording are separate. An enabled Tracker
owns open task records and message correlation even while recording is off.
A disabled helper returns after a nil check, allocates no task ID, and reads no
clock. Argument expressions are still evaluated by Go before the helper runs;
expensive optional arguments may need a caller-side guard. Tracking consumes
simulation IDs, so simulation decisions must not depend on numeric ID values.

## Component commands

`tasks.Owner` embeds only `naming.Named`: an owner supplies `Name() string`.
The simulation owns the tracker, clock, and ID allocator. Obtain the service with
`tracker := s.TaskTracker()`; component models also expose that same service
through `comp.TaskTracker()`. Owners need no service methods.

```go
id := tracker.StartTask(comp, tasks.StartInfo{
    ParentID: parent, Kind: tasks.KindPipeline, What: comp.Name() + ".bank",
})
tracker.AddTag(tasks.Tag{TaskID: id, What: "read-hit"})
tracker.AddMilestone(tasks.Milestone{
    TaskID: id, Kind: tasks.MilestoneKindWork, What: "bank",
})
tracker.EndTask(id)
```

`StartTask` allocates the ID and returns it; callers store it in component State.
Times are always stamped from the simulation clock held by the tracker. Zero means untracked. Zero or unknown
IDs on end/tag/milestone are ignored, including after checkpoint restoration.
`Kind`, `What`, and the owner name must be nonempty when tracking is enabled.
Tags and milestones allocate their own event ID if left zero.

Request helpers preserve separate sender and receiver identities:

```go
sendID := tracker.StartReqOut(comp, msg, parent) // before port.Send(msg)
recvID := tracker.StartReqIn(comp, msg)
tracker.EndReqIn(recvID)
tracker.EndReqOut(sendID)
```

Retain each returned handle for milestones, children, and completion. On reset,
close request tasks with `EndReqIn` / `EndReqOut` and other work with
`tracker.EndTask(id)`; zero and unknown handles are harmless.
Messages are unchanged. The tracker resolves request, response
buffer, and NoC transport parents once, before any observer filtering. Unknown
parents are zero; they are not filled in retroactively. A task can start at one
component and end at another component in the same simulation.

## Observers

```go
type Observer interface {
    TaskStarted(tasks.Task)
    TaskEnded(tasks.Task)
    TagAdded(tasks.Tag)
    MilestoneAdded(tasks.Milestone)
}
s.TaskTracker().Subscribe(observer)             // every component and port
s.TaskTracker().Subscribe(observer, component)  // this owner, including its ports
s.TaskTracker().Subscribe(observer, port)       // this port only
```

Subscribe at any time; scopes accept `naming.Named` values, including
interface-typed `messaging.Port`, and must use the same tracker. Filtering uses
actual ownership, not name prefixes. Subscribe before seeding work to see its
start. A later subscription receives future events, including full end records
for tasks already in flight, with no replay of earlier events. Average, total,
and busy-time tracers count the full received intervals, even when they began
before subscription or restore; busy time takes their union.
`tasks.NopObserver` supplies optional no-op callbacks. Both lifecycle callbacks
receive `Task`; at end it contains the original ID, MsgID, ParentID, kind, what,
location, start time, and end time. There is no `CompletedTask` type. `Detail`
is borrowed read-only data. Tags/milestones are separate notifications, not
history embedded in the completed record. Observers decide what to retain.

Notifications for one task are ordered. Different tasks may notify the same
observer concurrently; observers must synchronize their state. Callbacks must
not re-enter the tracker or call port buffer operations. The initial tracker
uses one mutex; port task commands take the port lock before the tracker lock,
followed by observer locks. Querying Owner/Name on a port is safe.

## Buffer identity

`twowaybuffered.Port` is a task owner with its own name and a private tracker
reference. `Simulation.RegisterPort` binds that reference to the simulation's
tracker once during assembly; component models register ports through `BindPort`.
The owning component needs no `TaskTracker()` method. Standalone ports can call
`BindTaskTracker(tracker)` explicitly before buffering messages; a nil or unbound
tracker disables tracing. The tracker supplies all timestamps and IDs.
Send/Deliver start a task and store its ID beside the message;
RetrieveIncoming/Outgoing end that task. Generic message hooks remain independent
and carry Msg in Item.

```go
msg, ok := port.PeekIncoming()
if ok && canAdmit(msg) {
    port.AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, "slots"))
    port.RetrieveIncoming()
}
// Connections can record an outgoing wait phase the same way:
port.AddOutgoingMilestone(tasks.Reason(tasks.MilestoneKindNetworkBusy, "link"))
```

Both milestone methods apply to the current head, recording the completed wait
since its previous milestone (or since it reached the head). They do not remove
the message. Call at each resolved wait-stage boundary, not on each retry tick;
multiple phases are supported. `messaging.WaitReason{}` selects the default.
Empty buffers, untracked messages, and zero-duration waits emit no milestone.

A queued message gets a `queue` milestone with the port name when it becomes
head, if it waited. Retrieval fills any remaining interval with `consumer` and
the component name for incoming, or `network_busy` and the connection name for
outgoing. An explicit milestone immediately before retrieval leaves no interval
to fill, so no duplicate fallback is emitted. Retrieve methods take no arguments.
The port privately checkpoints each task ID and last milestone time with its
message. An untracked restored head cannot be mistaken for a later tracked entry.

## Scope of this version

There is no public open-task snapshot or history replay. Starting DB recording mid-run does not promise to backfill all tasks
already in flight or their earlier tags/milestones. Completed basic records
remain self-contained, so recording can handle an end without retaining its
start.

When tracking is enabled, the simulation checkpoints open task records, owner
and component scope names, and request-parent correlation. Ports checkpoint
buffer task IDs and milestone boundaries with their messages. Loading replaces
that state; rebuilt hooks and fresh observers are retained. A restored task's
end carries its original ID, parent, and start time, even though the observer
never saw its start. `Detail` is omitted and is nil after restore. Tags,
milestones, and observer history are not checkpointed or replayed.

The tracker payload is optional: a checkpoint saved with tracking disabled and
restored with it enabled starts with an empty tracker; old unknown handles and
buffer messages are ignored. With tracking enabled at save and disabled at
restore, the payload is ignored. Other entities still require exact checkpoint
coverage. Restore into a rebuilt simulation with fresh observers. No
process-global task registry is used.

## Framework assembly

Task types, `Owner`, observers, and lifecycle implementations live directly in
this package. Tracker state, parent maps, and checkpoint data
structures are unexported. `Owner` provides identity only.

Frameworks construct `NewTracker(clock, newID, owns)` using the simulation's
clock, existing ID allocator, and a scope predicate that checks registered object
identity. This preserves scope isolation even if two simulations reuse names.
Components obtain the tracker from their simulation. Ports retain a private
tracker reference bound once by `Simulation.RegisterPort`, or explicitly through
`BindTaskTracker` for standalone ports. They expose no clock or ID methods and
require no tracker getter on their owner.
`Tracker.SaveCheckpoint` and `Tracker.LoadCheckpoint` operate while execution is
stopped. These are supported exported framework APIs. Ordinary applications use
the simulation builder, which owns checkpoint entity registration.
