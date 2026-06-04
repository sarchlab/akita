---
sidebar_position: 3
---

# Getting Started

The example you just ran in the previous chapter is the smallest possible
Akita simulation — one component that ticks three times and stops. This
chapter introduces what a component actually is, and walks through that
program line by line.

The source is in `examples/03_first_component/main.go`.

## What Is a Component?

A **component** is the default unit of behaviour in an Akita simulation.
Think of it as one piece of hardware — a core, a cache, a memory
controller, a network switch. Three things go into a component:

- **Spec** — immutable configuration. Things you set once when you build
  the component and never change at runtime: clock frequency, buffer
  sizes, latency values. A Go struct, JSON-serializable.
- **State** — mutable runtime data. Counters, queues, in-flight
  transactions, anything the component needs to remember between cycles.
  Also a Go struct, also JSON-serializable.
- **Middleware** — the behaviour. A struct with a `Tick() bool` method.
  Every cycle the engine calls `Tick`; the middleware reads the spec,
  mutates the state, optionally sends messages, and returns whether it
  made progress. A component can have several middlewares chained
  together; for now one is enough.

Components also have **ports** for talking to other components, but the
example in this chapter has none — it just prints on its own clock. Ports
arrive in the next chapter.

A component runs over **simulated time**. It declares a frequency (here 1
GHz) and the engine fires its `Tick` once per cycle in that clock domain.
Simulated time is in picoseconds (`timing.VTimeInSec`), so a 1 GHz cycle
is 1000 ps. The wall-clock time is whatever your CPU takes to evaluate
the `Tick` calls — usually much faster than the simulated time elapsed.

## Walk-Through

### 1. Spec and State

```go
type helloSpec struct {
    NumTicks int `json:"num_ticks"`
}

type helloState struct {
    Cycle int `json:"cycle"`
}
```

`helloSpec` is set once when the component is built — here, "tick three
times". `helloState` is what changes during the run: the current cycle
counter. The JSON tags mean the simulation is checkpoint-friendly without
any extra work from you.

### 2. The middleware

A middleware is anything with a `Tick() bool` method. A component can
have several, called in registration order every cycle, but one is enough
for this example.

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
  is work left, return `true`; once done, return `false`. When every
  component returns `false`, the engine's event queue empties and `Run`
  returns.

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

`AddMiddleware` registers the middleware that will run every cycle.

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

## Reading the Output

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

- **A component runs its middleware every cycle.** Spec is configuration,
  State is runtime data, middleware is the per-cycle work.
- **Both Spec and State are plain JSON-serializable structs.** That is
  what makes Akita simulations checkpoint-friendly out of the box.
- **Progress drives the loop.** Return `true` from `Tick` to be called
  again next cycle; return `false` to stop. Outside events (`TickLater`,
  incoming messages) can wake an idle component back up.
- **`modeling.None` is the sentinel for "no shared resources".** It is
  the third type parameter for components that do not reference shared
  state.

## Where to Next

This component talks to nobody. The next chapter introduces **ports** for
sending and receiving messages, **multiple middlewares** that share one
component's state, and the per-package **builder** convention used
throughout Akita.
