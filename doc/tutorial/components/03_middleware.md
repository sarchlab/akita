---
sidebar_position: 3
---

# Middleware: Per-Cycle Behaviour

A middleware is anything with a `Handle(e timing.Event) bool` method. A
component lists its middlewares as the fields of its **Middlewares**
struct, and every cycle it hands a tick to each of them in field order. One
is enough for the random walk:

```go
type Middlewares struct {
    // Walk takes one step per tick until the walker hits a wall.
    Walk *walkMW
}
```

The middleware itself:

```go
type walkMW struct {
    comp *Comp
}

func (m *walkMW) Handle(_ timing.Event) bool {
    state := &m.comp.State
    wall := m.comp.Spec().WallDistance

    if state.Position >= wall || state.Position <= -wall {
        fmt.Printf("hit wall at %+d after %d steps (%d ps)\n",
            state.Position, state.Steps, m.comp.CurrentTime())
        return false
    }

    if m.comp.Resources().RNG.Intn(2) == 0 {
        state.Position--
    } else {
        state.Position++
    }
    state.Steps++

    return true
}
```

Notice the field type `comp *Comp` — this is the alias from the previous
page standing in for
`*ticking.Component[Spec, State, Resources, Ports, Middlewares]`.

The component creates its middlewares itself, with a function that
receives the instance being built:

```go
func newMiddlewares(c *Comp) Middlewares {
    return Middlewares{Walk: &walkMW{comp: c}}
}
```

The next page hands this function to the component's `Definition`, and
`Build` calls it once for each instance.

A few things to notice:

- The middleware holds a reference to the component (`m.comp`) so it can
  read `Spec()` and `Resources()`, mutate `State`, and call
  `CurrentTime()`. That is the whole API surface for a minimal component.
- It holds **nothing else**. A middleware keeps only references;
  everything that changes during the run lives in `State`, because only
  `State` is saved in checkpoints. Even the random source is not a
  middleware field: it arrives through `Resources`, supplied by the system
  builder.
- The event argument is the tick, a `ticking.TickEvent`. A ticking
  middleware usually does its cycle of work without looking at it, so the
  walker names it `_`. (In the other two component models the same method
  receives other events; see *Wakeup and Event Components*.)
- The return value tells the component whether the middleware **made
  progress** this tick. Return `true` when it did something useful —
  advanced state, sent a message — and the component ticks again next
  cycle. Return `false` when it did nothing, either because there was no
  work or because it was blocked (for example, an outgoing port was full).
  When no middleware made progress, the component goes idle until something
  wakes it, such as an incoming message or a `TickLater`. Here the walker
  makes progress on every step, so it returns `true` until it reaches the
  wall and then returns `false`. When every component is idle, the engine's
  event queue empties and `Run` returns.

## Where to Next

The five structs and the middleware are all the pieces. The last page
declares the component's **Definition**, builds an instance, and runs the
simulation.
