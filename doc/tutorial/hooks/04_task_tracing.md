---
sidebar_position: 4
---

# Task Tracing

The hooks in the previous chapters fire at a single instant: *this event is
about to run*, *this message just left the port*, *the walker just stepped*.
That is perfect for logging, but it cannot answer the question you usually
care about: **how long did something take?** A duration needs two instants —
a start and an end — and a way to know they belong together.

That pairing is a **task**. Tracing is the layer that lets a component mark
the start and end of a unit of work; tools called **tracers** then turn those
marks into measurements. Tracing is built directly on the hook machinery you
already know — more on that in the last chapter.

The example for this chapter is in `examples/tracing/`.

## What You Will Learn

- Why a task (start + end) captures things a point-in-time hook cannot.
- How to open and close a task with `StartTask` and `EndTask`.
- How attaching a tracer turns those marks into a number.

## Why Tasks, Not Points

A hook tells you *something happened*. A task tells you *something happened
over a span of time*. With a task you can ask:

- How long did this request take, end to end?
- How much of the run was this component busy versus idle?
- How many of these did we process, and what was the average?

None of those are answerable from a single hook firing, because each needs
two points in time tied to the same piece of work. A task ties them together
with a shared **id**.

## Marking a Task

You mark the boundaries of a task with two calls:

```go
tracing.StartTask(domain, tracing.TaskStart{ID: id, ParentID: parentID, Kind: kind, What: what})
tracing.EndTask(domain, tracing.TaskEnd{ID: id})
```

- `domain` comes **first**. It is the component the task belongs to. It must
  be a component (or other `NamedHookable`), because these calls fire **task
  hooks** on it — that is the bridge back to the hook chapters. The time of
  the task is sourced from the domain automatically.
- `ID` uniquely identifies this task; `EndTask` matches the `StartTask` with
  the same `ID`. `ParentID` links it to a larger task (use `0` for none).
- `Kind` and `What` describe the task. Tracers select tasks by `Kind`, so
  give related tasks a common kind.

Two more calls fill in detail within a task, both optional:
`tracing.AddTaskTag(domain, tracing.TaskTag{...})` attaches a named tag, and
`tracing.AddMilestone(...)` records why a task was blocked and when it
unblocked. We will not need them here.

## Tasks Form a Tree

The `parentID` argument is what makes tasks more than a flat list. A task can
be the **child** of a larger task: a request being served may spawn a
sub-request, which spawns another, and so on. Recording each task's parent
turns them into a **tree** that mirrors how the work fanned out.

The worker in this chapter only has flat, top-level tasks, so it leaves
`ParentID` at its zero value. The tree becomes useful once work crosses
components — a
cache miss that goes to the next level down, for instance. Building a task
tree that spans a memory hierarchy is the subject of *Tracing Requests*.

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
        tracing.StartTask(m.comp, tracing.TaskStart{
            ID:   s.CurTaskID,
            Kind: "job",
            What: fmt.Sprintf("job-%d", s.CurTaskID),
        })

        s.Working = true
        s.CountDown = m.comp.Spec().CyclesPerJob
        s.JobsLeft--

        return true
    }

    s.CountDown--
    if s.CountDown == 0 {
        tracing.EndTask(m.comp, tracing.TaskEnd{ID: s.CurTaskID})
        s.Working = false
    }

    return true
}
```

With `NumJobs: 3` and `CyclesPerJob: 4` at 1 GHz, each job spans 4 cycles =
4000 ps, and the three run back to back.

## Seeing a Number

Marking tasks does nothing on its own — in fact, if no observer is attached,
`StartTask` and `EndTask` return immediately, so the cost is negligible. To
get a measurement you attach a **tracer**. The next chapter shows how to
write one; for now we use a built-in `BusyTimeTracer`, which reports how much
simulated time the component spent inside matching tasks:

```go
onlyJobs := func(t tracing.TaskStart) bool { return t.Kind == "job" }

busy := tracing.NewBusyTimeTracer(onlyJobs)
tracing.CollectTrace(worker, busy)
```

`CollectTrace(domain, tracer)` registers the tracer on the component — the
same idea as `AcceptHook`, which is exactly what it does under the hood. The
`func(tracing.TaskStart) bool` is a **filter**: the tracer only counts tasks
it returns true for. (Filters and the other built-in tracers get their own
chapter.)

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

(The example also attaches an `AverageTimeTracer`, hence the last two lines.)
Three jobs, each 4000 ps, so 12000 ps of busy time — measured entirely from
the outside. The worker's only tracing code is the `StartTask` / `EndTask`
pair; *what* to measure was decided at the call site by choosing a tracer.

## Key Concepts

- **A task is a unit of work with a start and an end**, tied together by a
  shared `id` — something a single-point hook cannot express.
- **Mark tasks with `StartTask` / `EndTask`** on a component (domain first);
  `AddTaskTag` and `AddMilestone` add optional detail.
- **A tracer turns marks into measurements.** Attach it with
  `tracing.CollectTrace(domain, tracer)`.
- **Tasks are free when unobserved.** With no tracer attached, the calls
  return immediately, so you can leave them in.

## Where to Next

A built-in tracer is a black box here. The next chapter opens it up: you will
**write your own tracer** by implementing the four-method `Tracer` interface.
