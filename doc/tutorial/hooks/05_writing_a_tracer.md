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
    StartTask(task Task)
    StepTask(task Task)
    AddMilestone(milestone Milestone)
    EndTask(task Task)
}
```

`CollectTrace` wires these up: it attaches a hook that calls `StartTask` when
a task starts, `EndTask` when it ends, and so on. Most tracers only care
about a couple of them and leave the rest as no-ops.

Note that the `Task` passed to `EndTask` only carries the task's `id` — the
framework does not remember the start time for you. A tracer that measures
duration keeps its own record between start and end, and reads the clock from
a `timing.TimeTeller`.

## A Max-Duration Tracer

Suppose we want the **longest** job the worker ran — there is no built-in for
that. Here is the whole tracer:

```go
type maxDurationTracer struct {
    timeTeller timing.TimeTeller
    starts     map[uint64]timing.VTimeInSec
    max        timing.VTimeInSec
}

func (t *maxDurationTracer) StartTask(task tracing.Task) {
    t.starts[task.ID] = t.timeTeller.CurrentTime()
}

func (t *maxDurationTracer) EndTask(task tracing.Task) {
    start, ok := t.starts[task.ID]
    if !ok {
        return
    }
    delete(t.starts, task.ID)

    if d := t.timeTeller.CurrentTime() - start; d > t.max {
        t.max = d
    }
}

// This tracer does not care about steps or milestones.
func (t *maxDurationTracer) StepTask(_ tracing.Task)          {}
func (t *maxDurationTracer) AddMilestone(_ tracing.Milestone) {}

func (t *maxDurationTracer) MaxDuration() timing.VTimeInSec { return t.max }
```

`StartTask` stamps the start time into a map keyed by task id; `EndTask`
looks it up, computes the span, and keeps the largest. `StepTask` and
`AddMilestone` are empty because this metric does not use them. The exported
`MaxDuration` is how the program reads the result afterward.

## Attaching It

A custom tracer attaches exactly like a built-in one:

```go
tracer := newMaxDurationTracer(engine)
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
  with only `StartTask`/`EndTask` doing real work.
- **`EndTask` gets only the id**, so a tracer that measures duration records
  start times itself and reads a `timing.TimeTeller` for the clock.
- **Custom and built-in tracers attach identically** with
  `tracing.CollectTrace`.

## Where to Next

So far our tasks have lived inside one component. Real work usually crosses
components — a request sent, handled, and answered. The next chapter traces
that **request lifecycle** end to end.
