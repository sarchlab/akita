---
sidebar_position: 5
---

# Writing a Tracer

The previous chapter attached a built-in `BusyTimeTracer` and treated it as a
black box. A tracer is not magic, though — it is any type that implements a
small interface. When the metric you want is not one of the built-ins,
you write your own.

The example is in `examples/customtracer/`.

## What You Will Learn

- The `tracing.Tracer` interface.
- How to implement a tracer that measures something the built-ins do not.
- That a custom tracer attaches with the same `CollectTrace` call.

## The Tracer Interface

A tracer has four methods, one per kind of task event:

```go
type Tracer interface {
    StartTask(task TaskStart)
    EndTask(task TaskEnd)
    AddTaskTag(tag TaskTag)
    AddMilestone(milestone Milestone)
}
```

`CollectTrace` wires these up: it attaches a hook that calls `StartTask` when
a task starts, `EndTask` when it ends, and so on. Most tracers only care
about a couple of them and leave the rest as no-ops. The simplest way to get
those no-ops is to embed `tracing.NopTracer`, which implements all four
methods as empty defaults — then you only override the ones you need.

Each event struct (`TaskStart`, `TaskEnd`, …) carries its own
`Time timing.VTimeInPicoSec`, so the framework hands you the time of every
event directly. A tracer that measures duration just records the start time
from `StartTask` and reads the end time from `EndTask` — no `timing.TimeTeller`
needed.

## A Max-Duration Tracer

Suppose we want the **longest** job the worker ran — there is no built-in for
that. Here is the whole tracer:

```go
type maxDurationTracer struct {
    tracing.NopTracer
    starts map[uint64]timing.VTimeInPicoSec
    max    timing.VTimeInPicoSec
}

func (t *maxDurationTracer) StartTask(task tracing.TaskStart) {
    t.starts[task.ID] = task.Time
}

func (t *maxDurationTracer) EndTask(task tracing.TaskEnd) {
    start, ok := t.starts[task.ID]
    if !ok {
        return
    }
    delete(t.starts, task.ID)

    if d := task.Time - start; d > t.max {
        t.max = d
    }
}

func (t *maxDurationTracer) MaxDuration() timing.VTimeInPicoSec { return t.max }
```

`StartTask` stamps the event's `Time` into a map keyed by task id; `EndTask`
looks it up, subtracts to get the span, and keeps the largest. There are no
`AddTaskTag` or `AddMilestone` methods here because the embedded
`tracing.NopTracer` already supplies them as no-ops. The exported
`MaxDuration` is how the program reads the result afterward.

## Attaching It

A custom tracer attaches exactly like a built-in one:

```go
tracer := newMaxDurationTracer()
tracing.CollectTrace(worker, tracer)
```

`CollectTrace` does not care whether the tracer is yours or Akita's — it only
needs the `Tracer` interface.

This example has no filter, so the tracer sees every task. (A real tracer
often takes a `TaskFilter` and ignores tasks it does not match, the way the
built-ins do — that is the next-but-one chapter.)

## Running It

The worker here makes each job longer than the last (4000, 8000, 12000 ps):

```bash
cd examples/customtracer
go run main.go
```

Output:

```
longest job: 12000 ps
```

## Key Concepts

- **A tracer is any type implementing `Tracer`** — four methods, most often
  with only `StartTask`/`EndTask` doing real work. Embed `tracing.NopTracer`
  to get the rest as no-ops.
- **Each event carries its own `Time`**, so a tracer that measures duration
  records the start time itself and subtracts it from the end time — no
  `timing.TimeTeller` needed.
- **Custom and built-in tracers attach identically** with
  `tracing.CollectTrace`.

## Where to Next

So far our tasks have lived inside one component. Real work usually crosses
components — a request sent, handled, and answered. The next chapter traces
that **request lifecycle** end to end.
