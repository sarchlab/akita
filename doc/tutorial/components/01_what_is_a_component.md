---
sidebar_position: 1
---

# What Is a Component?

The example you ran in *Install and Verify* — the random walk — is the
smallest useful Akita simulation: a single component that takes one random
step per cycle until it drifts to ±10 and stops. This section builds that
program from scratch, a few lines at a time, so you understand every part
of a component before you connect two of them in the next section.

The finished source is in `examples/03_random_walk/main.go`.

## The Three Parts

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
random walk has none — it just walks on its own clock. Ports arrive in the
next section, *Make Components Talk to Each Other*.

## The `Comp` Type Alias

In Akita a component's Go type is the generic
`modeling.Component[Spec, State, Resources]`. The three type parameters
are exactly the three parts above (the third, *Resources*, is for shared
state that this component does not use). Spelled out in full, that type is
a mouthful and appears in many places — the middleware, the builder, every
helper.

The convention throughout Akita is to alias it once per package:

```go
type Comp = modeling.Component[walkSpec, walkState, modeling.None]
```

`modeling.None` is the sentinel for "no shared resources". From here on we
write `Comp` (or `*Comp`) instead of the full generic. You will define
this alias in the next page, right after the `walkSpec` and `walkState`
types it refers to.

## Simulated Time

A component runs over **simulated time**. It declares a frequency (here
1 GHz) and the engine fires its `Tick` once per cycle in that clock
domain. Simulated time is measured in picoseconds, so a 1 GHz cycle is
1000 ps. The wall-clock time is whatever your CPU takes to evaluate the
`Tick` calls — usually far less than the simulated time elapsed.

## Where to Next

Next we define the two data structures every component starts with —
**Spec and State** — and the `Comp` alias that ties them together.
