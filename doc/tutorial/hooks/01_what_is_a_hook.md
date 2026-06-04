---
sidebar_position: 1
---

# What Is a Hook?

You now have components that run and talk to each other. The next thing you
will want is to *watch* them — log every event, count messages, measure how
long things take — without editing the component code. That is what
**hooks** are for.

A **hook** is a callback the framework invokes at defined points in a
running simulation. You attach a hook from the outside; the object you
attach it to calls your hook whenever it reaches one of those points. The
component never knows the hook is there.

The example for this and the next chapter is in `examples/hooks/`.

## What You Will Learn

- The `hooking` API: the `Hook` interface, `HookCtx`, and `AcceptHook`.
- What a **hook position** is and which objects expose them.
- How to attach an engine hook that logs every event.

## The Hook Interface

A hook is any type with a single method:

```go
type Hook interface {
    Func(ctx HookCtx)
}
```

When the framework reaches a hook point, it calls `Func` with a `HookCtx`
describing what just happened:

```go
type HookCtx struct {
    Domain Hookable    // the object that fired the hook (engine, port, …)
    Pos    *HookPos    // which hook point this is
    Item   interface{} // the thing the hook point is about (an event, a message)
    Detail interface{} // optional extra data
}
```

The two fields you read most are `Pos` — *where* the hook fired — and
`Item` — *what* it fired about. `Pos` is a pointer you compare against the
named positions a type publishes, and `Item` is an `interface{}` you
type-assert to the concrete thing.

## Hookable Objects and Hook Positions

Anything that fires hooks implements `Hookable`:

```go
type Hookable interface {
    AcceptHook(hook Hook)
    NumHooks() int
    Hooks() []Hook
}
```

Engines, ports, and components are all `Hookable` — they embed a
`hooking.HookableBase` that provides these methods. To start observing one,
you call `AcceptHook` with your hook.

Each `Hookable` type publishes the **hook positions** it fires. The engine
publishes two:

```go
var HookPosBeforeEvent = &hooking.HookPos{Name: "BeforeEvent"}
var HookPosAfterEvent  = &hooking.HookPos{Name: "AfterEvent"}
```

At `HookPosBeforeEvent` the engine is about to hand an event to its handler;
`ctx.Item` is the `timing.Event` it is about to process.

## A Logging Hook

Here is a hook that prints one line per event:

```go
type eventHook struct{}

func (h *eventHook) Func(ctx hooking.HookCtx) {
    if ctx.Pos != timing.HookPosBeforeEvent {
        return
    }
    evt := ctx.Item.(timing.Event)
    fmt.Printf("[event] t=%d handler=%s\n", evt.Time(), evt.HandlerID())
}
```

Two things to notice:

- The hook fires at **both** `HookPosBeforeEvent` and `HookPosAfterEvent`.
  We only want one line per event, so we return early unless this is the
  *before* position.
- `ctx.Item` is typed as `interface{}`, so we assert it to `timing.Event`
  to read `Time()` and `HandlerID()`.

Attaching it is one call. The simulation engine is `Hookable`, so:

```go
engine.AcceptHook(&eventHook{})
```

(`simulation`-built engines work the same way — `s.GetEngine()` returns a
`timing.Engine`, whose interface includes `AcceptHook`.)

## Running It

`examples/hooks/` builds two ticking agents — Agent A sends a ping to Agent
B, which replies — over a direct connection, then attaches the hook above.
The agents print nothing themselves; every line comes from the hook:

```
[event] t=1000 handler=AgentA
[event] t=1000 handler=Conn
[event] t=2000 handler=AgentA
[event] t=2000 handler=AgentB
[event] t=2000 handler=Conn
[event] t=3000 handler=AgentB
[event] t=3000 handler=Conn
[event] t=4000 handler=AgentB
[event] t=4000 handler=AgentA
[event] t=4000 handler=Conn
[event] t=5000 handler=AgentA
```

You are watching the engine's heartbeat: each agent ticking, and the
connection delivering messages between them — without a single `fmt.Print`
inside the agents.

## Key Concepts

- **A hook is a `Func(ctx HookCtx)` callback** the framework invokes at
  defined points.
- **`ctx.Pos` says where, `ctx.Item` says what.** Compare `Pos` against the
  named positions; type-assert `Item` to the concrete value.
- **Attach with `AcceptHook`.** Engines, ports, and components are all
  `Hookable`.
- **Hooks are non-invasive.** The observed component is unchanged and
  unaware.

## Where to Next

The engine is only one source of hooks. Next we attach a hook to a **port**
to watch the messages themselves.
