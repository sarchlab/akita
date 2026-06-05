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
func (m *walkMW) Tick() bool {
    s := &m.comp.State
    wall := m.comp.Spec().WallDistance

    if s.Position >= wall || s.Position <= -wall {
        return false
    }

    if m.rng.Intn(2) == 0 {
        s.Position--
    } else {
        s.Position++
    }
    s.Steps++

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

**The comma-ok form** is the safe version. Use it when you are not certain —
a position might carry different payloads, or you would simply rather skip
than crash:

```go
step, ok := ctx.Item.(walkStep)
if !ok {
    return
}
```

On a mismatch `ok` is `false` and `step` is the zero value, so the hook
quietly ignores anything it does not recognize.

**A type switch** is for when one hook legitimately handles several concrete
types. The message hook from *Hooking into Messages* is the natural case — a
port carries both requests and responses:

```go
switch msg := ctx.Item.(type) {
case *pingReq:
    // msg is a *pingReq in this branch
case *pingRsp:
    // msg is a *pingRsp in this branch
}
```

Each case binds `msg` to that branch's type, and a `default` catches
anything unexpected.

Rule of thumb: assert directly when the type is guaranteed, use comma-ok when
it is not, and reach for a type switch when more than one type is valid.

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

You have now seen hooks end to end — built-in points and your own. The last
chapter shows **tracing**, which builds on exactly this machinery to measure
how long work takes.
