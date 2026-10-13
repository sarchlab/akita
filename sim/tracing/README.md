# Tracing observers

`sim/tracing` contains observers for the simulation-scoped `sim/tasks` service.
See [the task API](../tasks/README.md) for component commands, registration,
concurrency, buffer identity, and checkpoint behavior.

```go
s := sim.MakeBuilder().WithTaskTracking().Build()
// Build components, then subscribe before seeding work or running the engine.
latency := tracing.NewAverageTimeTracer(func(t tasks.Task) bool {
    return t.Kind == tasks.KindReqIn
})
s.Tasks().Subscribe(latency, component)
```

`TaskStarted(Task)` and `TaskEnded(Task)` carry complete basic records, with
`EndTime` populated at completion. Time totals and averages need no start-time
map. `TagAdded(Tag)` and `MilestoneAdded(Milestone)` supply separate events.
Filters use `func(tasks.Task) bool`; nil selects all tasks. Time totals/averages
evaluate the filter at end, while busy-time and tag observers evaluate at start.
Use stable task properties (kind, what, location) in a shared filter.

## Built-in observers

| Observer | Purpose |
|---|---|
| AverageTimeTracer | Mean duration of selected completed tasks |
| TotalTimeTracer | Sum of selected task durations |
| BusyTimeTracer | Time covered by selected active tasks |
| TagCountTracer | Event counts and distinct task counts per tag |
| BackTraceTracer | Retains open tasks for parent-chain printing |
| DBTracer | Writes task records, tags, milestones, and recording segments |

## Database recording

The simulation subscribes its DBTracer when tracking is enabled. Monitor and
trace-on-start enable tracking automatically. `StartTracing` and `StopTracing`
control recording windows independently of tracking or other observers.

A task whose start is recorded is retained until completion. An end received
while recording is on is also recordable without its start: the end carries
its full identity and original start time. No snapshot or history backfill is
performed; a task spanning an entire recording window may be omitted if both
its start and end occurred outside that window. Earlier tags and milestones
are not recovered. While recording is off, DBTracer retains only event buffers
for work already selected by a recording window.

The unchanged SQLite schema uses `trace`, `milestone`, `tag`, and
`daisen$segments`. Task locations are interned in `location`. Tags/milestones
inherit location through their TaskID join. Milestone deduplication keeps the
first milestone at each timestamp for a task; the same kind/what at a later
time remains visible. IDs and parent IDs are numeric in Daisen.

## Task conventions

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
(incoming vs outgoing buffer). This holds for every task: `tasks.Start` derives the
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
`tasks.Start` parented to the `req_in`, normally `tasks.KindPipeline` — that spans
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
