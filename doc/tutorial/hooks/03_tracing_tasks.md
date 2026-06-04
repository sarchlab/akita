---
sidebar_position: 3
---

# Tracing Tasks

Raw hooks give you a firehose of low-level events. Usually you want
something higher-level: *how many requests did this component handle, and
how long did each take?* Akita answers that with **tracing**, which is built
directly on the hooks you just saw.

The example for this chapter is in `examples/tracing/`.

## What You Will Learn

- What a tracing **task** is, and how to open and close one.
- What a **tracer** is and how to attach one.
- How to read a measurement out of a built-in tracer.

## Tasks

A **task** is one unit of work with a beginning and an end — a request being
served, a job being processed, a packet in flight. You mark its boundaries
with two calls:

```go
tracing.StartTask(id, parentID, domain, kind, what, detail)
tracing.EndTask(id, domain)
```

- `id` uniquely identifies this task; `parentID` links it to a larger task
  (use `0` for none).
- `domain` is the component the task belongs to — it must be a component (or
  other `NamedHookable`), because `StartTask` and `EndTask` fire **task
  hooks** on it. This is the bridge to the previous chapters: tracing is just
  hooks at task granularity.
- `kind` and `what` describe the task; tracers filter on `kind`.

## Tracers

A **tracer** is a hook that consumes tasks. You do not write the hook
yourself — you create a built-in tracer and attach it with
`tracing.CollectTrace`:

```go
onlyJobs := func(t tracing.Task) bool { return t.Kind == "job" }

busy := tracing.NewBusyTimeTracer(engine, onlyJobs)
avg  := tracing.NewAverageTimeTracer(engine, onlyJobs)

tracing.CollectTrace(worker, busy)
tracing.CollectTrace(worker, avg)
```

- The first argument is a `timing.TimeTeller` so the tracer can stamp tasks
  with the current simulated time — the engine works (so does the component;
  both implement `TimeTeller`).
- The second is a `TaskFilter` (`func(Task) bool`); the tracer ignores tasks
  it does not match.
- `CollectTrace(domain, tracer)` registers the tracer's hook on the
  component, exactly like `AcceptHook` in the previous chapters.

Akita ships several tracers — `BusyTimeTracer`, `AverageTimeTracer`,
`TotalTimeTracer`, `StepCountTracer`, and a `DBTracer` that writes to a
database (covered later, in *Observability and Persistence*).

## The Worker

The component being measured is a small worker that processes a fixed number
of jobs, each taking a fixed number of cycles. It opens a task when a job
starts and closes it when the job finishes:

```go
func (m *workerMW) Tick() bool {
    s := &m.comp.State

    if !s.Working {
        if s.JobsLeft == 0 {
            return false
        }

        s.NextID++
        s.CurTaskID = s.NextID
        tracing.StartTask(
            s.CurTaskID, 0, m.comp,
            "job", fmt.Sprintf("job-%d", s.CurTaskID), nil)

        s.Working = true
        s.CountDown = m.comp.Spec().CyclesPerJob
        s.JobsLeft--

        return true
    }

    s.CountDown--
    if s.CountDown == 0 {
        tracing.EndTask(s.CurTaskID, m.comp)
        s.Working = false
    }

    return true
}
```

With `NumJobs: 3` and `CyclesPerJob: 4` at 1 GHz, each job spans 4 cycles =
4000 ps, and the three run back to back.

## Running It

```bash
cd examples/tracing
go run main.go
```

Output:

```
jobs traced:  3
busy time:    12000 ps
average time: 4000 ps
```

Three jobs, each 4000 ps, so 12000 ps of busy time and a 4000 ps average —
all measured by tracers attached from the outside. The worker's only
tracing-related code is the `StartTask` / `EndTask` pair; *which* metrics to
collect was decided entirely at the call site, by choosing tracers.

## Key Concepts

- **A task is a unit of work** marked by `tracing.StartTask` /
  `tracing.EndTask` on a component.
- **Tracing is hooks at task granularity.** `StartTask`/`EndTask` fire task
  hooks; tracers are the hooks that consume them.
- **A tracer measures; a filter selects.** Attach with
  `tracing.CollectTrace(domain, tracer)`, filtering by task `kind`.
- **Choose metrics at the call site.** The component emits tasks once;
  swapping tracers changes what you measure without touching it.

## Where to Next

You have now seen the full default toolkit: components, ports and messages,
and hooks and tracing to observe them. The next section drops below the
component layer to the **events** the engine schedules directly — the
primitive everything else is built on.
