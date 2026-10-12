# Tracing requests across components

A request has a sender task (`req_out`, from issue through response) and a
receiver task (`req_in`, from admission through completion). Their IDs are
independent of the message ID and each other. Components own those IDs in
checkpointed transaction State.

## Start and finish

All request helpers take the domain first. Start helpers return the task ID;
completion helpers take that ID rather than a message.

```go
type transaction struct {
    Req        messaging.Msg `json:"req"`
    RecvTaskID uint64        `json:"recv_task_id"`
    SendTaskID uint64        `json:"send_task_id"`
}

// Sender: check port capacity, start, then send.
tx.SendTaskID = tracing.TraceReqInitiate(comp, req, parentTaskID)
port.Send(req)
// After finding the transaction for the response:
tracing.TraceReqFinalize(comp, tx.SendTaskID)

// Receiver: retrieve/admit, then retain the ID in State.
tx.RecvTaskID = tracing.TraceReqReceive(comp, req)
tracing.AddMilestone(comp, tracing.Milestone{
    TaskID: tx.RecvTaskID, Kind: tracing.MilestoneKindData, What: "data",
})
tracing.TraceReqComplete(comp, tx.RecvTaskID)
```

The transaction above illustrates the fields; a leaf needs only its receive
ID, while a forwarding component retains both. A synchronous handler can keep
an ID in a local variable if its entire lifecycle completes within that call.
Start helpers return zero without hooks. A reset ends every task it discards
with `EndTaskOnReset(comp, taskID)`; there are no lookup or forget calls.

## Forwarding and parent links

A cache handling an upstream request supplies its stored receive ID as the
parent of the downstream send task:

```go
tx.RecvTaskID = tracing.TraceReqReceive(comp, upstream)
tx.SendTaskID = tracing.TraceReqInitiate(comp, downstream, tx.RecvTaskID)
bottom.Send(downstream)
```

The downstream receiver does not look up the sender's task. Both starts carry
`TaskStart.MsgID`, and an observer resolves that correlation into `ParentID`.
`DBTracer` and `BackTraceTracer` do this automatically. Custom tracers use one
`tracing.RequestTaskParents` per tracer, calling `ResolveStart(task)` before
recording a start and `End(task.ID)` for every end. The helper is not safe for
concurrent calls without the tracer's lock.

A sender the tracer never saw has no matching parent. This includes a sender
that is not traced and requests already in flight when a checkpoint is
restored. The receiver still has its own stable task ID; a fresh tracer cannot
reconstruct a task start from before restore. Observer correlation is never
checkpointed or used by the simulation.

## Admission and buffer tasks

Ports store the incoming and outgoing buffer task IDs beside their buffered
messages. Their checkpoints retain both. Buffer hooks report the actual head
transitions, so restored queues need no observer-side depth reconstruction.
Use the incoming head's task ID for the time spent waiting for admission:

```go
tracing.AddMilestone(comp, tracing.Milestone{
    TaskID: top.PeekIncomingTaskID(),
    Kind: tracing.MilestoneKindHardwareResource,
    What: comp.Name() + ".slots",
})
req, ok := top.RetrieveIncoming()
```

Read the ID before retrieving. Zero means the queue is empty or the entry
was enqueued without tracing. The buffer task ends at retrieval, where the
component starts its receive task. Do not attach milestones to a control
message unless the component actually opened a task for it.

## Runnable examples

- `go run ./examples/reqtracing` reports request round-trip and handling time.
- `go run ./examples/tasktree` prints the linked Client → L1 → L2 → Memory
  task tree using the reusable parent tracker.

See [the tracing reference](../../../sim/tracing/README.md) for event types,
buffer hook details, and checkpoint behavior, and [V5 release notes](../../release-notes-v5.md#tracing-task-ownership)
for the mechanical downstream migration.
