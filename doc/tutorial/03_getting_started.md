---
sidebar_position: 3
---

# Getting Started

The example you just ran in the previous chapter is the smallest useful
Akita simulation — a single component that takes one random step per
cycle until it drifts to ±10 and stops. This chapter introduces what a
component actually is, and walks through that program line by line.

The source is in `examples/03_random_walk/main.go`.

## What Is a Component?

A **component** is the default unit of behaviour in an Akita simulation.
Think of it as one piece of hardware — a core, a cache, a memory
controller, a network switch. Three things go into a component:

- **Spec** — immutable configuration. Things you set once when you build
  the component and never change at runtime: clock frequency, buffer
  sizes, thresholds. A Go struct, JSON-serializable.
- **State** — mutable runtime data. Counters, queues, in-flight
  transactions, anything the component needs to remember between cycles.
  Also a Go struct, also JSON-serializable.
- **Middleware** — the behaviour. A struct with a `Tick() bool` method.
  Every cycle the engine calls `Tick`; the middleware reads the spec,
  mutates the state, optionally sends messages, and returns whether it
  made progress. A component can have several middlewares chained
  together; for now one is enough.

Components also have **ports** for talking to other components, but the
example in this chapter has none — it just walks on its own clock. Ports
arrive in the next section.

A component runs over **simulated time**. It declares a frequency (here
1 GHz) and the engine fires its `Tick` once per cycle in that clock
domain. Simulated time is in picoseconds (`timing.VTimeInSec`), so a
1 GHz cycle is 1000 ps. The wall-clock time is whatever your CPU takes
to evaluate the `Tick` calls — usually much faster than the simulated
time elapsed.

## Walk-Through

### 1. Spec and State

```go
type walkSpec struct {
    WallDistance int `json:"wall_distance"`
}

type walkState struct {
    Position int `json:"position"`
    Steps    int `json:"steps"`
}
```

`walkSpec` is set once when the component is built — here, "stop when
the walker drifts 10 units from the origin in either direction".
`walkState` is what changes during the run: where the walker is, and how
many steps it has taken. The JSON tags mean the simulation is
checkpoint-friendly without any extra work from you.

### 2. The middleware

A middleware is anything with a `Tick() bool` method. A component can
have several, called in registration order every cycle, but one is
enough for this example.

```go
type walkMW struct {
    comp *modeling.Component[walkSpec, walkState, modeling.None]
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

A few things to notice:

- The middleware holds a reference to the component (`m.comp`) so it can
  read `Spec()`, mutate `State`, and call `CurrentTime()`. That is the
  whole API surface for a minimal component.
- It also holds its **own** state — the random source (`rng`). Middleware
  is just a regular Go struct, so it can carry anything that does not
  need to survive checkpointing.
- The return value controls whether the engine reschedules. While there
  is work left, return `true`; once the wall is hit, return `false`. When
  every component returns `false`, the engine's event queue empties and
  `Run` returns.

### 3. Building the component

```go
s := simulation.MakeBuilder().Build()

comp := modeling.NewBuilder[walkSpec, walkState, modeling.None]().
    WithEngine(s.GetEngine()).
    WithFreq(1 * timing.GHz).
    WithSpec(walkSpec{WallDistance: 10}).
    Build("Walker")
comp.AddMiddleware(&walkMW{
    comp: comp,
    rng:  rand.New(rand.NewSource(1)),
})
```

`modeling.NewBuilder` is the generic constructor for a `Component[Spec,
State, Resources]`. The third type parameter is for **shared resources**,
which this component does not use — `modeling.None` is the sentinel for
"no resources". Set the engine, the clock frequency, and the spec, then
build with a unique name.

`AddMiddleware` registers the middleware that will run every cycle. The
RNG is seeded with `1` so the output is reproducible.

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
hit wall at +10 after 52 steps (53000 ps)
```

The walker happened to take 52 steps before drifting 10 units away from
zero — that is one realisation of a random walk. With seed 1 the
trajectory is fixed, so every run prints exactly this line. Change the
seed and the path (and the step count) changes.

The simulated time, 53000 ps = 53 ns, equals 53 cycles at 1 GHz — that
is the 52 walking steps plus one for the `TickLater` that started the
clock. Try setting the frequency to 100 MHz and you will see 530 ns
instead, with the step count unchanged. That is the whole point of a
discrete-event simulator: the model's behaviour is defined by cycles, and
the clock just dictates how those cycles map to simulated time.

## Key Concepts

- **A component runs its middleware every cycle.** Spec is configuration,
  State is runtime data, middleware is the per-cycle work.
- **Both Spec and State are plain JSON-serializable structs.** That is
  what makes Akita simulations checkpoint-friendly out of the box.
- **Progress drives the loop.** Return `true` from `Tick` to be called
  again next cycle; return `false` to stop. Outside events (`TickLater`,
  incoming messages) can wake an idle component back up.
- **Middleware is just a Go struct.** It can carry whatever auxiliary
  data the behaviour needs (here, a random source).
- **`modeling.None` is the sentinel for "no shared resources".** It is
  the third type parameter for components that do not reference shared
  state.

## Where to Next

This component talks to nobody. The next chapter introduces **ports** for
sending and receiving messages, **multiple middlewares** that share one
component's state, and the per-package **builder** convention used
throughout Akita.
