---
sidebar_position: 8
---

# How Tracing Builds on Hooks

This last chapter is optional — you can use tracing without it — but it ties
the section together. Everything in the tracing chapters is the **hook**
mechanism from the start of this section, specialized around a small set of
task event payloads (`TaskStart`, `TaskEnd`, and friends). Once you see that,
nothing about tracing is mysterious.

## `StartTask` Fires a Hook

Recall *Defining Your Own Hook Point*: a component exposes internal behavior
by filling a `HookCtx` and calling `InvokeHook`. `tracing.StartTask` does
exactly that. Stripped to its essence:

```go
func StartTask(domain NamedHookable, t TaskStart) {
    if domain.NumHooks() == 0 {
        return
    }

    if t.Location == "" {
        t.Location = domain.Name()
    }
    t.Time = domain.CurrentTime()

    domain.InvokeHook(hooking.HookCtx{
        Domain: domain,
        Item:   t,
        Pos:    HookPosTaskStart,
    })
}
```

The payload (`ctx.Item`) is a `TaskStart` value; the position is the
predefined `HookPosTaskStart`. `EndTask`, `AddTaskTag`, and `AddMilestone` are
the same shape with `HookPosTaskEnd`, `HookPosTaskTag`, and
`HookPosMilestone`, carrying a `TaskEnd`, `TaskTag`, or `Milestone`
respectively. So a component that traces tasks is just a component firing its
own hook points — the technique you already learned — at four standard
positions.

The `NumHooks() == 0` guard is why tasks are free when nothing is observing:
with no tracer attached there are no hooks, and the call returns before
stamping the time or invoking anything.

## A Tracer Is a Hook

On the other side, `CollectTrace` is a thin wrapper over `AcceptHook`. It
attaches an internal hook, `traceHook`, that translates hook positions into
`Tracer` method calls:

```go
func CollectTrace(domain NamedHookable, tracer Tracer) {
    domain.AcceptHook(&traceHook{t: tracer})
}

func (h *traceHook) Func(ctx hooking.HookCtx) {
    switch ctx.Pos {
    case HookPosTaskStart:
        h.t.StartTask(ctx.Item.(TaskStart))
    case HookPosTaskTag:
        h.t.AddTaskTag(ctx.Item.(TaskTag))
    case HookPosMilestone:
        h.t.AddMilestone(ctx.Item.(Milestone))
    case HookPosTaskEnd:
        h.t.EndTask(ctx.Item.(TaskEnd))
    }
}
```

This is the `ctx.Pos` switch and `ctx.Item` type assertion from *Hooking into
Messages* and *Defining Your Own Hook Point*, doing one specific job: routing
each task event to the matching method of the `Tracer` interface — note that
each position carries its own payload type (`TaskStart`, `TaskTag`,
`Milestone`, `TaskEnd`). Your `maxDurationTracer` from chapter 5 never touched
`hooking` — `traceHook` did the hook work and handed it clean event values.

## The Whole Picture

```text
component:  tracing.StartTask(...)  --->  InvokeHook(HookPosTaskStart, TaskStart)
                                                  |
                                          traceHook.Func
                                                  |
tracer:                                   Tracer.StartTask(TaskStart)
```

Tracing, then, is a **convention** layered on hooks: standard hook positions
(`HookPosTaskStart` and friends), a standard set of payloads (`TaskStart`,
`TaskEnd`, `TaskTag`, `Milestone`), and a standard hook (`traceHook`) that
adapts them to the `Tracer` interface. Hooks are the general mechanism for
getting information out of a simulation; tracing is the specialization for
measuring work over time.

## Key Concepts

- **`StartTask`/`EndTask` are `InvokeHook` calls** at the standard positions
  `HookPosTaskStart` / `HookPosTaskEnd`, carrying a `TaskStart` / `TaskEnd` as
  the item.
- **`CollectTrace` is `AcceptHook`** of a built-in `traceHook` that dispatches
  each task hook position (`HookPosTaskStart`, `HookPosTaskTag`,
  `HookPosMilestone`, `HookPosTaskEnd`) to the matching `Tracer` method.
- **Tracing is hooks plus a convention** — standard positions, standard
  payloads, and a standard adapter hook.

## Where to Next

That completes the toolkit for observing a simulation. The next section drops
below the component layer to the **events** the engine schedules directly —
the primitive that components, hooks, and tracing are all built on.
