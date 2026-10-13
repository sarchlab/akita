# Simulation tasks

Enable tracking explicitly with `sim.MakeBuilder().WithTaskTracking()`.
`WithVisTracingOnStart` and `WithMonitor` also enable it. `s.Tasks()` and
`s.VisTracer()` are nil otherwise, and no tracing tables are created.
`WithTaskTracking` alone creates and subscribes the database tracer without
starting recording; call `s.VisTracer().StartTracing()` to begin. Engine-only
examples can use
`modeling.NewStandaloneSimulation(engine).WithTaskTracking()` before assembly.

Tracking, subscribing, and database recording are separate. An enabled Tracker
owns open task records and message correlation even while recording is off.
A disabled helper returns after a nil check, allocates no task ID, and reads no
clock. Argument expressions are still evaluated by Go before the helper runs;
expensive optional arguments may need a caller-side guard. Tracking consumes
simulation IDs, so simulation decisions must not depend on numeric ID values.

## Component commands

Every component model supplies `tasks.Owner` through ComponentBase. Custom
owners provide `Name`, `CurrentTime`, `NewID`, and `TaskTracker`; no Hookable is
needed. The returned Tracker must belong to the same simulation as the clock
and ID allocator.

```go
id := tasks.Start(comp, tasks.StartInfo{
    ParentID: parent, Kind: tasks.KindPipeline, What: comp.Name() + ".bank",
})
tasks.AddTag(comp, tasks.Tag{TaskID: id, What: "read-hit"})
tasks.AddMilestone(comp, tasks.Milestone{
    TaskID: id, Kind: tasks.MilestoneKindWork, What: "bank",
})
tasks.End(comp, tasks.EndInfo{ID: id})
```

`Start` allocates the ID and returns it; callers store it in component State.
`EndInfo` currently contains only `ID`, leaving room for optional fields.
Times are always stamped from the owner. Zero means untracked. Zero or unknown
IDs on end/tag/milestone are ignored, including after checkpoint restoration.
`Kind`, `What`, and the owner name must be nonempty when tracking is enabled.
Tags and milestones allocate their own event ID if left zero.

Request helpers preserve separate sender and receiver identities:

```go
sendID := tasks.StartReqOut(comp, msg, parent) // before port.Send(msg)
recvID := tasks.StartReqIn(comp, msg)
tasks.CompleteReq(comp, recvID)
tasks.FinalizeReq(comp, sendID)
```

Retain each returned handle for milestones, children, completion, and
`EndOnReset`. Messages are unchanged. The tracker resolves request, response
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
s.Tasks().Subscribe(observer)             // every component and port
s.Tasks().Subscribe(observer, component)  // this owner, including its ports
```

Subscribe before the first engine run. Subscribe before seeding work to observe
its start as well as subsequent notifications.
`tasks.NopObserver` supplies optional no-op callbacks. Both lifecycle callbacks
receive `Task`; at end it contains the original ID, MsgID, ParentID, kind, what,
location, start time, and end time. There is no `CompletedTask` type. `Detail`
is borrowed read-only data. Tags/milestones are separate notifications, not
history embedded in the completed record. Observers decide what to retain.

Notifications for one task are ordered. Different tasks may notify the same
observer concurrently; observers must synchronize their state. Callbacks must
not re-enter the tracker or call port buffer operations. The initial tracker
uses one mutex; port hooks take the port lock before the tracker lock, followed
by observer locks. Querying Owner/Name on a port is safe.

## Buffer identity

Ports have no tracing methods or task IDs. Their four message hooks carry a
`messaging.Msg` Item and `messaging.PortBufferChange` Detail. Detail reports the
actual head after the operation, whether it exists, and whether it changed.
The tracker converts these into buffer tasks keyed by port name, message ID, and
direction, and resolves a queue milestone only for the actual new head.

```go
msg, ok := port.PeekIncoming()
if ok && canAdmit(msg) {
    tasks.AddBufferMilestone(comp, port, msg, tasks.Milestone{
        Kind: tasks.MilestoneKindHardwareResource, What: "slots",
    })
    port.RetrieveIncoming()
}
```

Admission names the message, never a FIFO offset or peeked task ID. An
untracked head A followed by tracked B cannot cause A's milestones to attach
to B. Normal retrieval ends the task; it is not a reset.

## Scope of this version

There is no public open-task snapshot, history replay, or live subscription
handoff. Starting DB recording mid-run does not promise to backfill all tasks
already in flight or their earlier tags/milestones. Completed basic records
remain self-contained, so recording can handle an end without retaining its
start.

When tracking is enabled, the simulation checkpoints open task records, owner
names, buffer identities, and request-parent correlation. Loading replaces that
state; rebuilt port hooks and fresh observers are retained. A restored task's
end carries its original ID, parent, and start time, even though the observer
never saw its start. `Detail` is omitted and is nil after restore. Tags,
milestones, and observer history are not checkpointed or replayed.

The tracker payload is optional: a checkpoint saved with tracking disabled and
restored with it enabled starts with an empty tracker; old unknown handles and
buffer messages are ignored. With tracking enabled at save and disabled at
restore, the payload is ignored. Other entities still require exact checkpoint
coverage. Restore into a rebuilt simulation with fresh observers. No
process-global task registry is used. Tracker construction, port wiring,
subscription freezing, and checkpoint serialization are framework internals,
not public methods on `tasks.Tracker`.
