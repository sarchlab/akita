# Task tracing

A task describes work over simulated time. Components use `sim/tasks`; observers
use `sim/tasks/tracing` or implement `tasks.Observer`. Enable tracking on the simulation
builder before component assembly, then subscribe before seeding work or running.

```go
s := sim.MakeBuilder().WithTaskTracking().Build()
// Build worker with s.
busy := tracing.NewBusyTimeTracer(func(t tasks.Task) bool { return t.Kind == "job" })
s.TaskTracker().Subscribe(busy, worker)
```

The worker starts a job with `tasks.Start` and stores the returned ID in State:

```go
state.CurTaskID = tasks.Start(comp, tasks.StartInfo{Kind: "job", What: "compute"})
// Later, after doing the work:
tasks.End(comp, tasks.EndInfo{ID: state.CurTaskID})
```

The helper allocates an independent task ID and stamps simulated time. It returns
zero when tracking is disabled; end/tag/milestone calls accept zero as a no-op.
Do not use task ID values to control job length or simulation behavior.

`AddTag` attaches categorical labels such as read-hit. `AddMilestone` marks when
a blocking condition resolves. A task has one location and an optional parent.
Observers receive the complete basic record on both start and end, with end time
filled at completion; they choose what events to retain.

Run [examples/tracing](../../../examples/tracing) for busy and average time, or
read the [full task contracts](../../../sim/tasks/README.md).
