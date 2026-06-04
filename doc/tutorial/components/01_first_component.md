---
sidebar_position: 1
---

# Your First Component

The smallest possible Akita component: one component, no ports, one
middleware that prints the current cycle. The component runs for a few
cycles, then stops.

The full source is in `examples/03_first_component/main.go`.

## What You Will Learn

- The mental model of an Akita component: a clocked unit whose work
  happens each cycle.
- The three parts you always write: **Spec**, **State**, and a middleware
  with a `Tick() bool` method.
- How `TickLater` and `Tick`'s return value drive the cycle-by-cycle loop.

## The Mental Model

A component has a clock frequency. Every cycle, the engine delivers a tick
event; the component runs its middleware list and decides whether to
schedule itself for the next cycle. The decision is the return value of
`Tick`:

- `return true` — "I made progress, ask me again next cycle."
- `return false` — "Nothing to do, do not reschedule."

When no component asks for a next tick and no other events are pending, the
engine's queue empties and the simulation ends.

## Walk-Through

### 1. Spec and State

Spec is immutable configuration; State is mutable runtime data. Both are
plain Go structs with JSON tags, so the simulation is checkpoint-friendly
without you doing anything extra.

```go
type helloSpec struct {
    NumTicks int `json:"num_ticks"`
}

type helloState struct {
    Cycle int `json:"cycle"`
}
```

`helloSpec` is set once when the component is built (here, "tick three
times"). `helloState` is what changes during the run (here, the current
cycle counter).

### 2. The middleware

A middleware is anything with a `Tick() bool` method. A component can have
several middlewares — each is called in registration order every cycle —
but one is enough for now.

```go
type helloMW struct {
    comp *modeling.Component[helloSpec, helloState, modeling.None]
}

func (m *helloMW) Tick() bool {
    state := &m.comp.State
    if state.Cycle >= m.comp.Spec().NumTicks {
        return false
    }

    fmt.Printf("tick %d at %d ps\n", state.Cycle, m.comp.CurrentTime())
    state.Cycle++

    return true
}
```

Two things to notice:

- The middleware holds a reference to the component (`m.comp`) so it can
  read `Spec()`, mutate `State`, and call `CurrentTime()`. That is the
  whole API surface for a minimal component.
- The return value controls whether the engine reschedules. While there
  is work left, return `true`; once done, return `false`.

### 3. Building the component

```go
s := simulation.MakeBuilder().Build()

comp := modeling.NewBuilder[helloSpec, helloState, modeling.None]().
    WithEngine(s.GetEngine()).
    WithFreq(1 * timing.GHz).
    WithSpec(helloSpec{NumTicks: 3}).
    Build("Hello")
comp.AddMiddleware(&helloMW{comp: comp})
```

`modeling.NewBuilder` is the generic constructor for a `Component[Spec,
State, Resources]`. The third type parameter is for **shared resources**,
which this component does not use — `modeling.None` is the sentinel for
"no resources". Set the engine, the clock frequency, and the spec, then
build with a unique name.

`AddMiddleware` registers the middleware that will run every cycle. The
order matters once you have several; here there is only one.

### 4. Kicking it off

```go
comp.TickLater()

if err := s.GetEngine().Run(); err != nil {
    panic(err)
}

s.Terminate()
```

`TickLater` schedules the first tick at the next cycle. Without it the
engine has no events to fire and `Run` returns immediately. Once a tick
fires, the middleware returns `true` and the component reschedules itself
automatically — you only call `TickLater` once to start it (and again
later if the component went idle and now has work to do).

## Run It

```bash
cd examples/03_first_component
go run main.go
```

Output:

```
tick 0 at 1000 ps
tick 1 at 2000 ps
tick 2 at 3000 ps
```

At 1 GHz a cycle is 1000 ps. `TickLater` scheduled the first tick at
cycle 1; the next two come from the middleware returning `true`; the
fourth tick would have happened, but the middleware returned `false`, so
the queue emptied and `Run` returned.

## Key Concepts

- **Components run their middleware every cycle.** The middleware's `Tick`
  method is the cycle-by-cycle work.
- **Spec is configuration, State is runtime data.** Both are plain
  JSON-serializable structs.
- **Progress drives the loop.** Return `true` from `Tick` to be called
  again next cycle; return `false` to stop. Outside events (`TickLater`,
  incoming messages) can wake an idle component back up.
- **`modeling.None` means "no shared resources".** It is the third type
  parameter for components that do not reference shared state.

## Where to Next

This component talks to nobody. The next chapter introduces **ports** for
sending and receiving messages, **multiple middlewares** that share one
component's state, and the per-package **builder** convention used
throughout Akita.
