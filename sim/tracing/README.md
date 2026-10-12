# tracing

Package `tracing` provides a task-based tracing framework for observing what
happens inside a simulation. It uses the `hooking` mechanism to collect
structured traces without modifying component logic.

## Core Concepts

### Tasks

A `Task` is the aggregate record a stateful tracer builds from the stream of
trace events. It has a start and end time, a location (see [Locations](#locations)),
an optional parent, and lists of tags and milestones.

Components do not build a `Task` directly. They emit lightweight event structs;
the emit functions stamp the event time from the domain's clock and fire a hook.

### Tags

A `TaskTag` is a categorical label attached to a task while it is processed —
e.g. `"read-hit"`, `"write-miss"`. (Tags were previously called "steps".)

### Milestones

A `Milestone` records when a blocking condition is resolved during task
processing — the interval from the previous milestone (or the task start) to a
milestone is the time the task spent blocked on that reason. A milestone's
location is inherited from its task.

Each milestone carries a `MilestoneKind`:

| Kind | Marks the resolution of |
|------|-------------------------|
| `MilestoneKindHardwareResource` | a wait for a hardware resource (buffer slot, bank, MSHR) |
| `MilestoneKindNetworkTransfer` | a network transfer of the message |
| `MilestoneKindNetworkBusy` | a wait to send on a busy port |
| `MilestoneKindQueue` | a wait behind other messages in a queue/buffer |
| `MilestoneKindData` | a wait for a data response |
| `MilestoneKindDependency` | a wait on an ordering/dependency (e.g. in-order commit) |
| `MilestoneKindTranslation` | a wait for an address translation |
| `MilestoneKindSubTask` | a wait on a child subtask |
| `MilestoneKindWork` | the end of a **working** (not blocked) interval — see below |
| `MilestoneKindOther` | anything else |

`MilestoneKindWork` is the exception to the "milestone = blocking resolved"
reading: it marks the end of an interval the component spent doing productive
work rather than waiting — e.g. traversing an internal latency pipeline. The
interval up to a `work` milestone is time spent working, not blocked. Per the
coverage principles below, a `work` (or `subtask`) milestone must be paired with
a child subtask that spans the interval.

## Locations

**One location, one kind.** A location is the row a viewer (Daisen) draws tasks
on, and a row is read as a single sequential timeline. So every location must
host exactly **one** Kind of task. When a single component or port runs more than
one kind, the kinds overlap in time on one row and the timeline becomes
ambiguous — therefore the location string is qualified to separate them:

| Kind | Location | Example |
|------|----------|---------|
| `req_in` | `<component>.req_in` | `L2Cache.req_in` |
| `req_out` | `<component>.req_out` | `L2Cache.req_out` |
| `pipeline` | `<component>.<stage>` (the task's `What`) | `L2Cache.dir_pipeline`, `L2Cache.bank` |
| `incoming_buffer` | `<port>.incoming` | `L2Cache.Top.incoming` |
| `outgoing_buffer` | `<port>.outgoing` | `L2Cache.Bottom.outgoing` |

The rule of thumb: a **component** disambiguates by *stage* (the request side it
is on, or the named pipeline stage); a **port** disambiguates by *direction*
(incoming vs outgoing buffer). This holds for every task: `StartTask` derives the
location from the Kind (see `singleKindLocation`) when a caller leaves `Location`
empty, the buffer hooks append the direction, and all of these strings are
interned into the trace's `location` table.

When you add a new task Kind to a component or port that already emits another
kind, qualify its location the same way — never let two kinds share one location.

## Coverage principles

A `req_in` processing task's milestones and subtasks must, together, account for
the task's **entire** lifetime. Two rules keep traces honest — a reviewer should
never have to ask "why is the bar still running with no reason?" or "what work is
this, and why so long?".

**P1 — Full coverage (no unexplained gaps).** Every interval between a task's
start and end is attributed: the span ending at each milestone is the time the
task spent on that milestone's reason, so consecutive milestones tile the whole
lifetime. In particular the *last* milestone must land at (within ~one cycle of)
the task end. A gap between the last milestone and the end means a processing
phase is going unrecorded — the bar runs on with no reason. Emit a milestone for
that phase (or open a subtask for it, per P2).

**P2 — Work is a subtask (`work`/`subtask` ⇒ child subtask).** A `work` or
`subtask` milestone asserts the task spent that interval doing internal work
rather than blocked. Such an interval must be backed by a **child subtask** — a
`StartTask` parented to the `req_in`, normally `PipelineTaskKind` — that spans
it, so the trace shows *what* the work was, not merely that time elapsed. A bare
`work`/`subtask` milestone with no corresponding child subtask is a violation:
either open the subtask, or — if the interval was really a wait — reclassify the
milestone (`data`, `dependency`, `hardware_resource`, …).

So a pipelined component pairs each internal-latency phase with **both** a
subtask bar (the child) and a `work` milestone (on the parent) at the phase's
end — see *Pipeline subtasks*. The write-through cache is the reference
implementation: its directory-lookup and data-array (bank) latencies each open a
`pipeline` subtask under the `req_in` and close with a matching `work` milestone,
so they render as labelled child bars instead of unexplained gaps, and the only
bare spans left are the inherent single-cycle buffer-admission and response ticks.

## Emit API

The domain is always the first argument; the per-event data goes in a struct.
Callers pass **no** time — the emit functions read it from the domain's clock,
and only when a tracer is attached (the `NumHooks()==0` fast path keeps tracing
free when disabled). A domain must therefore be a `NamedHookable`: a
`naming.Named`, `hooking.Hookable`, and `timing.TimeTeller` that also has
`InvokeHook` and allocates IDs with `NewID() uint64`. Every component built
from a component model is one.

```go
tracing.StartTask(domain, tracing.TaskStart{
    ID: id, ParentID: parentID, Kind: "req_in", What: "ReadReq", Detail: msg,
})
tracing.AddTaskTag(domain, tracing.TaskTag{TaskID: id, What: "read-hit"})
tracing.AddMilestone(domain, tracing.Milestone{
    TaskID: id, Kind: tracing.MilestoneKindQueue, What: "queued",
})
tracing.EndTask(domain, tracing.TaskEnd{ID: id})
```

`TaskStart.Location` is optional: when left empty `StartTask` derives a
single-kind location from the Kind (`singleKindLocation` — see
[Locations](#locations)), so `req_in`/`req_out`/`pipeline` are each placed on
their own row. Set it explicitly to override (e.g. network tracing, or the port
buffer hooks that append a direction). `StartTask` requires a non-empty `ID`,
`Kind`, and `What` (and a named domain) or it panics; tag and milestone `ID`s are
auto-generated when left zero.

### Message tracing and task ownership

Task IDs are independent of message IDs. Start helpers allocate with
`domain.NewID()` and return the task ID (zero if the domain has no hooks):

```go
sendTaskID := tracing.TraceReqInitiate(domain, msg, parentTaskID)
recvTaskID := tracing.TraceReqReceive(domain, msg)
tracing.TraceReqComplete(domain, recvTaskID)
tracing.TraceReqFinalize(domain, sendTaskID)
```

The sender and receiver each retain their own returned ID in their transaction
`State`. Use that ID for milestones, tags, child tasks, completion, and
`EndTaskOnReset(domain, taskID)`. Zero IDs are ignored by end, tag, and milestone
helpers. The message is unchanged. There is no registry or lookup/forget step.
Start a sender task **before** `port.Send(msg)` so observers see the request
before its outgoing-buffer child.

`TaskStart.MsgID` identifies the message handled by a task. `req_in` starts
have no simulation-supplied parent. `DBTracer` and `BackTraceTracer` each own a
`RequestTaskParents`, which records `MsgID -> req_out task ID`, resolves parents
on receive/buffer/message-transport starts, and deletes the mapping on req_out end. Buffer tasks
for responses resolve using `Detail.(messaging.Msg).RspTo`; their `MsgID` still
identifies the response itself. NoC `msg_e2e` tasks resolve the same way.
Explicit parents of other task kinds are
preserved. Custom tracers that need these links use the same helper:

```go
// Under the tracer's lock, before filtering or retaining a start:
task = parents.ResolveStart(task)
// For every end, including tasks not recorded by the tracer:
parents.End(task.ID)
```

The helper's zero value is usable. It is observer state, is not checkpointed,
and never affects simulation decisions. If the observer missed the sender
start (sender not traced, or request already in flight across restore), the
receiver/buffer has `ParentID == 0`. Events are not retroactively reparented.
Share one tracer across the domains whose task trees should be linked; use a
separate tracer for each simulation.

### Reset and checkpoints

A component that drops work ends every owned task with
`EndTaskOnReset(domain, taskID)` before discarding its transaction. The same
helper handles receive, send, pipeline, and other task kinds. An observer
ignores an end for a task whose start it did not see.

Task IDs in component State and port entries survive checkpoints. Restoring
emits no new starts: subsequent milestones and ends use the saved IDs. A fresh
observer cannot reconstruct tasks that began before it attached, but an
observer retaining the original starts can match the restored ends.

### Port buffer tracing

`CollectIncomingBufferTrace(port)` and `CollectOutgoingBufferTrace(port)` attach
stateless hooks. `twowaybuffered.Port` stores each buffered message together
with its task ID and serializes both via the message codec. Its hook events
keep `messaging.Msg` as `Item` and supply `messaging.PortBufferTrace` in `Detail`:
`TaskID` is the affected entry and `HeadTaskID` is the entry that just reached
the head (zero if none). Hooks must not call buffer operations (peek, count, or mutate) while processing
these events, because the port may hold its buffer lock.

The incoming task spans delivery to retrieval at `<port>.incoming`; outgoing
spans send to drain at `<port>.outgoing`. Both emit a queue milestone when the
entry reaches the head. Queue position comes directly from port state, so
restoring needs no observer-side queue-depth counter.

Components attach admission milestones to the head of the incoming buffer:

```go
tracing.AddMilestone(domain, tracing.Milestone{
    TaskID: port.PeekIncomingTaskID(),
    Kind: tracing.MilestoneKindHardwareResource,
    What: domain.Name() + ".slots",
})
msg, ok := port.RetrieveIncoming()
```

Read the task ID **before** retrieving. `PeekIncomingTaskID() uint64` returns
zero for an empty buffer or an entry enqueued without tracing. Peeks do not
reserve the head; the component remains the sole incoming-buffer consumer.
The buffer task closes at retrieve, where the component starts its own req_in.
Old checkpoints with bare message arrays must be recreated for the new entry
format. Daisen's task/parent schema is unchanged.

### Pipeline subtasks

A component with an internal latency pipeline between retrieve and processing
(e.g. the TLB or a cache directory pipeline) records the traversal as a subtask
of kind `PipelineTaskKind` (`"pipeline"`), parented to its `req_in`, opened at
pipeline entry (the tick `req_in` opens, at retrieve) and closed at pipeline
exit. Without it the pipeline latency would be an unattributed gap between the
buffer task (ends at retrieve) and the first post-pipeline `req_in` milestone.

## Tracer Interface

Each method receives the event struct for its trace point:

```go
type Tracer interface {
    StartTask(t TaskStart)
    EndTask(t TaskEnd)
    AddTaskTag(tag TaskTag)
    AddMilestone(m Milestone)
}
```

Embed `tracing.NopTracer` to implement only the methods you care about. Events
carry their own `Time`, so tracers do not need a `TimeTeller`.

### Connecting a Tracer to a Domain

```go
tracing.CollectTrace(domain, tracer)
```

## Built-in Tracers

| Tracer | Purpose |
|--------|---------|
| `AverageTimeTracer` | Average task duration for filtered tasks |
| `TotalTimeTracer` | Total task processing time |
| `BusyTimeTracer` | Non-overlapping busy time (merges overlapping tasks) |
| `TagCountTracer` | Counts how many times each tag name occurs |
| `BackTraceTracer` | Records in-flight tasks; `DumpBackTrace` prints the parent chain |
| `DBTracer` | Persists tasks, tags, and milestones to a `DataRecorder` (SQLite) |

The time/count tracers take only a `TaskFilter` (`func(TaskStart) bool`):

```go
t := tracing.NewTotalTimeTracer(func(t tracing.TaskStart) bool {
    return t.Kind == "req_in"
})
```

### DBTracer

```go
tracer := tracing.NewDBTracer(timeTeller, dataRecorder)
tracer.StartTracing()
// ... simulation runs ...
tracer.StopTracing()
tracer.Terminate()
```

Only tasks that overlap an active tracing window are recorded. `DBTracer` keeps
a `TimeTeller` solely to time-stamp the tracing-window segments. It writes four
tables:

- `trace` — one row per task. `Location` is dictionary-encoded via the shared
  `location` table (`akita_data:"location"`) to keep the largest table small.
- `milestone` — one row per milestone (no location; inherited from the task).
- `tag` — one row per tag (no location; inherited from the task).
- `daisen$segments` — one row per `StartTracing`/`StopTracing` window, so a
  reader knows which time ranges were captured.

**Milestone de-duplication.** When recording, `DBTracer` keeps only the *first*
milestone for a given `(Kind, What)` on a task, and drops any later milestone
whose timestamp equals one already recorded on that task. This collapses retried
emissions and zero-duration stalls automatically — so a milestone you emit may
not appear in the DB if an equivalent one is already there.

## Hook Positions

| Position | When |
|----------|------|
| `HookPosTaskStart` | Task begins |
| `HookPosTaskTag` | Tag recorded |
| `HookPosMilestone` | Milestone recorded |
| `HookPosTaskEnd` | Task ends |
