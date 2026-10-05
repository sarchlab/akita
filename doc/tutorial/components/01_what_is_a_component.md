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

## The Five Structs

A **component** is the default unit of behaviour in an Akita simulation.
Think of it as one piece of hardware — a core, a cache, a memory
controller, a network switch. A component type is defined by five Go
structs, all declared in the component's package:

- **Spec** — the configuration. Things you set once when you build the
  component and never change at runtime: clock frequency, buffer sizes,
  thresholds. Scalars and slices of scalars, JSON-serializable.
- **State** — mutable runtime data. Counters, queues, in-flight
  transactions, anything the component needs to remember between cycles.
  Also JSON-serializable, and saved in checkpoints. Only the component's
  own code writes it.
- **Resources** — references to objects shared with the rest of the
  simulation: a backing storage, a page table, or, for the random walk, a
  random number source.
- **Ports** — one field per port, for talking to other components. The
  random walk has none; ports arrive in the next section, *Make Components
  Talk to Each Other*.
- **Middlewares** — the behaviour. One field per middleware, and a
  middleware is a struct with a `Handle(e timing.Event) bool` method. Every
  cycle the component hands a tick to each middleware in field order; the
  middleware reads the Spec, mutates the State, optionally sends messages,
  and returns whether it made progress. For now one middleware is enough.

A package-level variable named `Definition` ties the five together: it
holds the default Spec and the function that creates the middlewares. The
component's package declares the type; it does not build instances.

## The System Builder

The code that assembles a simulation — usually `main`, or a test — is the
**system builder**. It builds each component instance from the component's
`Definition`, choosing its Spec, supplying its Resources, and creating its
ports. For the walker, that is a few lines in `main`:

```go
walker := Definition.Builder().
    WithSimulation(s).
    WithResources(Resources{RNG: rand.New(rand.NewSource(1))}).
    Build("Walker")
```

Keep that split in mind: the component decides *what* it is — its structs
and its behaviour — and the system builder decides how many instances
exist, how each is configured, and what it is connected to. The last page
of this section covers building in detail.

## Three Component Models

The five structs are the same for every component, but Akita has three
**component models** — three ways a component can be run — each in its own
sub-package of `modeling`:

| Model | Runs when | Use it for |
|---|---|---|
| `modeling/ticking` | every cycle, while a middleware makes progress | work that advances cycle by cycle: pipelines, caches, switches. The default. |
| `modeling/wakeup` | a port has activity, or at a time it asked for | a component that is idle most of the time and knows when it next has work |
| `modeling/event` | an event arrives: port activity, or an event it scheduled | behaviour that is a set of distinct happenings, each with its own data |

This section and the next use ticking components throughout. The other two
models are covered in
[Wakeup and Event Components](../events/03_event_driven.md), once you have
seen the events they are built on.

## The `Comp` Type Alias

A ticking component's Go type is the generic
`ticking.Component[Spec, State, Resources, Ports, Middlewares]`. The five
type parameters are exactly the five structs above. Spelled out in full,
that type is a mouthful and appears in many places — the middleware, the
`Definition`, every helper.

The convention throughout Akita is to alias it once per package:

```go
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
```

From here on we write `Comp` (or `*Comp`) instead of the full generic. You
will define this alias on the next page, right after the structs it refers
to.

## Simulated Time

A ticking component runs over **simulated time**. Its Spec declares a
clock frequency (here 1 GHz), and the component handles one tick per cycle
in that clock domain. Simulated time is measured in picoseconds, so a
1 GHz cycle is 1000 ps. The wall-clock time is whatever your CPU takes to
run the middlewares — usually far less than the simulated time elapsed.

## Where to Next

Next we define the data structs every component starts with — **Spec,
State, Resources, and Ports** — and the `Comp` alias that ties them
together.
