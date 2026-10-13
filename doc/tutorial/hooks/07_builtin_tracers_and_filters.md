---
sidebar_position: 7
---

# Built-in Tracers and Filters

You have now written a tracer and seen a couple of built-in ones in passing.
This chapter surveys the tracers Akita ships and, just as importantly, the
**filter** — the function that decides which tasks a tracer pays attention
to. The filter is what lets one simulation, emitting many kinds of task,
answer many different questions.

## The Filter Comes First

Every measuring tracer takes a `TaskFilter`:

```go
type TaskFilter func(t tasks.Task) bool
```

The tracer calls it when a task **starts**; it only measures tasks for which
the filter returns `true`. There are no special helpers — it is an ordinary
function, so you select on whatever field of the `tasks.Task` matters:

```go
// By kind — the most common choice.
func(t tasks.Task) bool { return t.Kind == "req_in" }

// By the message/operation name.
func(t tasks.Task) bool { return t.What == "readReq" }

// By where the task ran (the component, or a network location).
func(t tasks.Task) bool { return t.Location == "L1Cache" }

// Combine freely.
func(t tasks.Task) bool {
    return t.Kind == "req_in" && t.What == "readReq"
}

// Everything (when the domain only emits what you want).
func(t tasks.Task) bool { return true }
```

This is why the request example could read two different numbers from one
run: the same tasks flowed past both tracers, and each tracer's filter kept
only its kind — `req_out` for the round trip, `req_in` for handling.

## The Time Tracers

Three tracers measure time and differ only in how they combine tasks. Each
takes a filter **only** — they read timestamps from the task records they receive, so there is no `TimeTeller` argument:

- **`NewAverageTimeTracer(filter)`** — the mean duration of matching tasks.
  Read it with `AverageTime()`; `TotalCount()` gives how many finished. Use it
  for "how long does an average request take?"
- **`NewBusyTimeTracer(filter)`** — the wall-clock time during which *at least
  one* matching task was open. Overlapping tasks collapse into one interval,
  so this is **utilization**: "how much of the run was this component doing
  something?" Read it with `BusyTime()`.
- **`NewTotalTimeTracer(filter)`** — the sum of every matching task's
  duration, counting overlaps multiple times. Read it with `TotalTime()`.
  Divide by `BusyTime()` for average concurrency.

For a component that never overlaps tasks (like the worker), busy and total
time are equal; for one that handles many requests at once, they diverge, and
the gap tells you how parallel it is.

## The Tag Tracer

**`NewTagCountTracer(filter)`** counts the categorical **tags** you attach to
a task with `tracing.AddTaskTag` (for example `"read-hit"` or `"write-miss"`).
It needs no `TimeTeller` — it counts, not times. `TagNames()`,
`TagCount(name)`, and `TaskCount(name)` tell you how often each tag was
recorded and how many distinct tasks carried it — handy for seeing which path
through a component is hot.

## The Back-Trace Tracer

**`NewBackTraceTracer(printer)`** is a debugging aid. It keeps the tasks that
*started but never ended* and can print their parent chains — exactly what
you want when a simulation deadlocks and you need to know which request is
stuck and what it was waiting on.

## The DB Tracer

The tracers above keep a single aggregate in memory. **`DBTracer`** instead
records *every* task, tag, and milestone to a database for offline analysis.
Unlike the time tracers it does take a `timing.TimeTeller` (the engine works),
alongside a `datarecording.DataRecorder`:

```go
recorder := datarecording.NewDataRecorder("trace")
dbTracer := tracing.NewDBTracer(engine, recorder)
simulation.Tasks().Subscribe(dbTracer, component)

// ... run the simulation ...

dbTracer.Terminate() // flush to disk
```

It writes a SQLite file you can query afterward, and supports
`StartTracing()` / `StopTracing()` to capture only a window of interest
(full traces get large fast). Recording a full trace this way — then querying
it, or loading it into a visualizer — is how you analyze a large simulation
offline; here we just place the tracer in the lineup.

## Choosing One

| Want | Tracer |
|---|---|
| Average request latency | `AverageTimeTracer` |
| Utilization / busy fraction | `BusyTimeTracer` |
| Average concurrency | `TotalTimeTracer` ÷ `BusyTimeTracer` |
| Which tags occur, how often | `TagCountTracer` |
| What is stuck in a deadlock | `BackTraceTracer` |
| A full, queryable trace | `DBTracer` |

In every case the **filter** is what narrows a noisy simulation down to the
tasks behind your question.

## Key Concepts

- **The filter selects; the tracer measures.** A `func(tasks.Task) bool` over
  `Kind` / `What` / `Location` selects task records; the tracer turns them
  into a number.
- **Time tracers differ by overlap handling** — average, busy (union), and
  total (sum) answer different questions. They take a filter only and read the
  timestamps from task records.
- **`TagCountTracer` counts tags; `BackTraceTracer` finds stuck tasks;
  `DBTracer` records everything** to SQLite for offline analysis.

## Where to Next

The final chapter is an advanced look under the hood: how typed task notifications relate to generic port hooks.

Enable task tracking before assembly. Recording is separate from tracking; see
[recording windows and history limits](../../../sim/tasks/tracing/README.md).
