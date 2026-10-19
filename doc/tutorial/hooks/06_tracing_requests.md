# Observing request tasks

Request tasks use the typed `sim/tasks` API. Generic hooks remain available for
other instrumentation, but a task observer subscribes to the simulation service.

```go
s := sim.MakeBuilder().WithTaskTracking().Build()
// Assemble components and ports.
latency := tracing.NewAverageTimeTracer(func(t tasks.Task) bool {
    return t.Kind == tasks.KindReqOut
})
s.TaskTracker().Subscribe(latency, client)
```

Start the sender task before sending the message, and keep its returned handle
in component State. At the receiver, keep a separate receive handle:

```go
tracker := comp.TaskTracker()
state.SendTaskID = tracker.StartReqOut(comp, msg, parentID)
port.Send(msg)
// Receiver admission:
state.RecvTaskID = tracker.StartReqIn(comp, msg)
// Receiver completion:
tracker.EndReqIn(state.RecvTaskID)
// Sender receives the response:
tracker.EndReqOut(state.SendTaskID)
```

When forwarding, pass the upstream receive handle as the new sender task's
parent. The Tracker resolves the cross-component link once for all observers.
The message ID and task ID are independent. Disabled tracking returns zero,
which every completion/milestone helper treats as a no-op.

A custom observer can embed `tasks.NopObserver` and override `TaskEnded(Task)`;
it receives the complete basic task record, including both times, without
having to save every start. Callbacks may be concurrent for different tasks
and must not re-enter the Tracker or port buffer operations.

See the runnable [request example](https://github.com/sarchlab/akita/tree/main/examples/reqtracing),
[task tree](https://github.com/sarchlab/akita/tree/main/examples/tasktree), and [custom observer](https://github.com/sarchlab/akita/tree/main/examples/customtracer).
The [task API guide](../../../sim/tasks/README.md) covers buffer admission,
subscription scope, recording windows, and checkpoint restoration and history limits.
