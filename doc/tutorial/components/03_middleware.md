---
sidebar_position: 3
---

# Middleware: Per-Cycle Behaviour

A middleware is anything with a `Tick() bool` method. A component can have
several, called in registration order every cycle, but one is enough for
the random walk.

```go
type walkMW struct {
    comp *Comp
    rng  *rand.Rand
}

func (m *walkMW) Tick() bool {
    state := &m.comp.State
    wall := m.comp.Spec().WallDistance

    if state.Position >= wall || state.Position <= -wall {
        fmt.Printf("hit wall at %+d after %d steps (%d ps)\n",
            state.Position, state.Steps, m.comp.CurrentTime())
        return false
    }

    if m.rng.Intn(2) == 0 {
        state.Position--
    } else {
        state.Position++
    }
    state.Steps++

    return true
}
```

Notice the field type `comp *Comp` — this is the alias from the previous
page standing in for `*modeling.Component[walkSpec, walkState,
modeling.None]`.

A few things to notice:

- The middleware holds a reference to the component (`m.comp`) so it can
  read `Spec()`, mutate `State`, and call `CurrentTime()`. That is the
  whole API surface for a minimal component.
- It also holds its **own** state — the random source (`rng`). Middleware
  is just a regular Go struct, so it can carry anything that does not need
  to survive checkpointing.
- The return value tells the engine whether the middleware **made progress**
  this tick. Return `true` when it did something useful — advanced state, sent
  a message — and the engine ticks the component again next cycle. Return
  `false` when it did nothing, either because there was no work or because it
  was blocked (for example, an outgoing port was full); the engine then lets
  the component go idle until something wakes it, such as an incoming message
  or a `TickLater`. Here the walker makes progress on every step, so it returns
  `true` until it reaches the wall and then returns `false`. When every
  component is idle, the engine's event queue empties and `Run` returns.

## Where to Next

Spec, State, and middleware are all the pieces. The last page wires them
into a component with a **builder** and runs the simulation.
