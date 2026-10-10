---
sidebar_position: 4
---

# Building and Running

With the five structs and the middleware defined, two steps remain: the
component declares its `Definition`, and `main` — the system builder —
builds an instance from it and runs the engine.

## Declaring the Definition

```go
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
    DefaultSpec:    Spec{Freq: 1 * timing.GHz, WallDistance: 10},
    NewMiddlewares: newMiddlewares,
}
```

`Definition` is the component type's one public declaration: its default
configuration and its behaviour. Its type parameters are the five structs
again, in the same order as in `Comp`.

- `DefaultSpec` is the starting configuration. A system builder that wants
  something different copies it, changes fields, and passes the result in.
- `NewMiddlewares` is the function from the previous page; `Initialize` calls it
  to create each instance's middlewares.

An optional third field, `NewState`, returns an instance's initial State.
The walker omits it and starts from the zero State: position 0, no steps.
Keep the literal simple — keyed fields, constant values, and named
functions — because tooling reads the `Definition` without running your
code.

## Building the Component

```go
s := sim.MakeBuilder().Build()

walker := Definition.Builder().
    WithSimulation(s).
    WithResources(Resources{RNG: rand.New(rand.NewSource(1))}).
    Build("Walker")
```

`Definition.Builder()` returns a builder for this component type that
starts from `DefaultSpec`, and `Build` returns a `*Comp`. Set the simulation
the instance belongs to, supply its Resources — here a random source seeded
with `1`, so the output is reproducible — and build with a unique name. The
walker keeps the default Spec, so there is no `WithSpec` call, and it has no
ports, so there are no port bindings.

`Build` validates the Spec and State shapes and registers the configured
instance. It leaves State and Middlewares unset. After all components and
connections are built and bound, call `s.Initialize()` once. Initialization
validates wiring, freezes topology, and runs `NewState` and `NewMiddlewares`.

:::info The builder pattern

`Definition.Builder().WithSimulation(…).WithResources(…).Build("Walker")` is
the **builder pattern**: rather than one constructor with a long argument
list, you assemble the component through a chain of small `WithX` calls and
finish with `Build`. It reads top to bottom, and each setting is
independent.

**How the chaining works.** Every `WithX` takes the builder *by value*, sets
one field on its copy, and returns that copy. Because each call returns a
builder, the next one hangs off it. Nothing is mutated in place, so a
partially configured builder is just a value you can stash and reuse.

**Akita's convention.** Every component, from this walker to the DRAM
controller, is built with the same shape:
`Definition.Builder().WithSimulation(…).WithSpec(…).WithResources(…).Build(name)`.
The whole configuration goes in as one `Spec` through `WithSpec` (there is
no setter per Spec field), starting from `Definition.DefaultSpec`. That is
why the clock frequency lives in the Spec: `Build` reads `Freq` from it. The
ports are assigned individually with `component.BindPort("Field", port)` after
Build; the next section shows the system builder creating and binding them.

:::

## Kicking It Off

```go
if err := s.Initialize(); err != nil {
    panic(err)
}
walker.TickLater()

if err := s.Engine().Run(); err != nil {
    panic(err)
}

s.Terminate()
```

`TickLater` schedules the first tick at the next cycle. Nothing wakes a
component with no ports, so without it the engine has no events to fire and
`Run` returns immediately. Once a tick fires, the middleware returns `true`
and the component schedules its next tick automatically — you only call
`TickLater` once to start it (and again later if the component went idle
and now has work to do).

## Reading the Output

```
hit wall at +10 after 52 steps (53000 ps)
```

The walker happened to take 52 steps before drifting 10 units away from
zero — that is one realisation of a random walk. With seed 1 the trajectory
is fixed, so every run prints exactly this line. Change the seed and the
path (and the step count) changes.

The simulated time, 53000 ps = 53 ns, equals 53 cycles at 1 GHz — that is
the 52 walking steps plus one for the `TickLater` that started the clock.
Try building the walker with a 100 MHz clock — copy
`Definition.DefaultSpec`, set its `Freq` to `100 * timing.MHz`, and pass it
with `WithSpec` — and you will see 530 ns instead, with the step count
unchanged. That is the whole point of a discrete-event simulator: the
model's behaviour is defined by cycles, and the clock just dictates how
those cycles map to simulated time.

## Key Concepts

- **A component is five structs and a `Definition`.** Spec is
  configuration, State is runtime data, Resources are references to shared
  objects, Ports talk to other components, and Middlewares are the
  behaviour.
- **A ticking component runs its middlewares every cycle**, in field order,
  while any of them makes progress.
- **Spec and State are plain JSON-serializable structs.** That is what
  makes Akita simulations checkpoint-friendly out of the box.
- **`type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]`**
  is the per-package alias that keeps the long generic out of the rest of
  the code. `modeling.None` fills the Resources slot when there are no
  shared objects.
- **The system builder builds instances** with
  `Definition.Builder()…Build(name)`; the component's package only declares
  the type.
- **Progress drives the loop.** Return `true` from `Handle` to tick again
  next cycle; return `false` to stop. Outside events (`TickLater`, incoming
  messages) can wake an idle component back up.
- **Middlewares hold only references.** Data that changes belongs in State;
  external objects, like the random source, come in through Resources.

## Where to Next

This component talks to nobody. The next section, **Make Components Talk to
Each Other**, introduces **ports** for sending and receiving messages,
**multiple middlewares** that share one component's state, and a component
in **its own package**, built twice by a system builder.
