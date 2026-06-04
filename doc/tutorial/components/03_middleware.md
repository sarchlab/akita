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
- The return value controls whether the engine reschedules. While there is
  work left, return `true`; once the wall is hit, return `false`. When
  every component returns `false`, the engine's event queue empties and
  `Run` returns.

## Where to Next

Spec, State, and middleware are all the pieces. The last page wires them
into a component with a **builder** and runs the simulation.
