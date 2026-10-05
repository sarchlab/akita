---
sidebar_position: 3
---

# Defining Your Own Hook Point

The hooks so far attached to points Akita already provides — engine events
and port messages. Those are useful, but they only show *framework-level*
activity. They cannot see what happens **inside** your component: a cache
line being evicted, a queue filling up, a walker taking a step. To let
observers watch your component's own behavior, you define your **own hook
point** and fire it yourself.

The example is in `examples/customhook/`.

## What You Will Learn

- How to declare a `HookPos` of your own.
- How to fire it from inside a component with `InvokeHook`.
- That a component is `Hookable`, so external code can observe it the same
  way it observes engines and ports.

## Declare a Hook Position

A hook position is just a named value. Declare one at package scope so both
the component (which fires it) and observers (which match on it) can refer
to the same value:

```go
var HookPosStep = &hooking.HookPos{Name: "WalkStep"}
```

It is compared by pointer identity, so there is exactly one of these and
everyone refers to it by name.

## Decide on a Payload

When the hook fires, observers read `ctx.Item` to learn what happened.
Define whatever type carries the information you want to expose:

```go
type walkStep struct {
    Position int
    Steps    int
}
```

## Fire the Hook

The component here is the random walker from *Create a Component*, taking one
±1 step per cycle. After each step it fires `HookPosStep`, passing the new
position as the `Item`:

```go
func (m *walkMW) Handle(_ timing.Event) bool {
    s := &m.comp.State
    wall := m.comp.Spec.WallDistance

    if s.Position >= wall || s.Position <= -wall {
        return false
    }

    if m.comp.Resources.RNG.Intn(2) == 0 {
        s.Position--
    } else {
        s.Position++
    }
    s.Steps++

    // Fire our own hook point. Anything that has accepted a hook on this
    // component now sees the step.
    m.comp.InvokeHook(hooking.HookCtx{
        Domain: m.comp,
        Pos:    HookPosStep,
        Item:   walkStep{Position: s.Position, Steps: s.Steps},
    })

    return true
}
```

`InvokeHook` is the other half of `AcceptHook`: every component embeds the
same `hooking.HookableBase` that engines and ports do, so it can both accept
hooks and invoke them. You fill in a `HookCtx` — `Domain` is the component
itself, `Pos` is your position, `Item` is your payload — and every attached
hook gets called. If no hook is attached, `InvokeHook` does almost nothing,
so the cost of leaving hook points in place is negligible.

## Observe It

An observer is an ordinary hook. Because more than one position can fire on
the same component, it checks `ctx.Pos` before acting, then asserts
`ctx.Item` to the payload type:

```go
type stepLogger struct{}

func (h *stepLogger) Func(ctx hooking.HookCtx) {
    if ctx.Pos != HookPosStep {
        return
    }
    step := ctx.Item.(walkStep)
    fmt.Printf("[step %d] position %+d\n", step.Steps, step.Position)
}
```

Attaching it is the same `AcceptHook` call as before — a component is
`Hookable`:

```go
walker.AcceptHook(&stepLogger{})
```

## Reading the Item

`ctx.Item` is an `interface{}`, so a hook has to recover the concrete type
before it can use it. Go gives you three ways, and the right one depends on
how sure you are about what `Item` holds.

**Type assertion** is the shortest form. Use it when you have already
narrowed things down — you checked `ctx.Pos` and you know exactly one payload
fires there:

```go
step := ctx.Item.(walkStep)
```

This is what `stepLogger` does. It **panics** if `Item` is not a `walkStep`,
which is fine here: only `HookPosStep` carries this payload, and we returned
early for every other position.

**The comma-ok form** is appropriate when the contract permits several types
and the hook intentionally observes only a subset. For example, after checking
a message hook position and extracting its `messaging.Msg`, a request-only
observer can filter the payload:

```go
req, ok := msg.Payload.(pingReq)
if !ok {
    return // Responses, other protocols, and nil payloads are not observed here.
}
```

This is a filter, not a way to hide a broken hook contract. At `HookPosStep`,
an item that is not a `walkStep` is an error; at a port message position, an
item that is not a `messaging.Msg` is an error. Those mismatches should panic.

**A type switch** handles several allowed payload types. A message observer
can inspect both requests and responses while retaining the routing fields:

```go
switch ctx.Pos {
case messaging.HookPosPortMsgSend, messaging.HookPosPortMsgRecvd:
    // Both positions guarantee that Item is a messaging.Msg.
default:
    return
}

msg := ctx.Item.(messaging.Msg)
switch payload := msg.Payload.(type) {
case pingReq:
    // payload is a pingReq; msg.ID and msg.Src remain available.
case pingRsp:
    // payload is a pingRsp.
default:
    // This observer ignores other valid payload types, including nil.
}
```

A general message tracer should still record nil payloads as `metadata`.
Optional tracing capabilities may be absent: a port owner that does not
support tracing, or has no trace hooks attached, does not need to produce
trace events.

Rule of thumb: ignore unrelated positions first, assert the types promised
by a matching position, and use payload checks to select the valid messages
an observer is interested in.

## Running It

```bash
cd examples/customhook
go run main.go
```

With the wall three units from the origin and a fixed RNG seed, the walker
drifts straight to +3:

```
[step 1] position +1
[step 2] position +2
[step 3] position +3
```

The walker prints nothing itself — every line is the external hook reading a
payload the component chose to expose. You decide exactly what internal
moments are observable and what data they carry.

## Key Concepts

- **Built-in hook points show the framework; your own hook points show your
  component.** Declare a `HookPos` to expose internal behavior.
- **`InvokeHook` fires a hook point.** Fill a `HookCtx` with `Domain`,
  `Pos`, and an `Item` payload; every attached hook is called.
- **Components are `Hookable`** — they both `AcceptHook` and `InvokeHook`,
  exactly like engines and ports.
- **Idle hook points are cheap.** With no hooks attached, firing one costs
  almost nothing, so you can leave them in production code.

## Where to Next

You have now seen hooks end to end — built-in points and your own. The next
chapters cover **tracing**, which builds on exactly this machinery to measure
how long work takes, starting with what a task is and why you need one.
