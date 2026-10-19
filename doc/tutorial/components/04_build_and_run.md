---
sidebar_position: 4
---

# Building and Running

With the five structs and the middleware defined, two steps remain: the
component declares its `DefaultSpec()` function, and `main` — the system builder —
builds an instance from it and runs the engine.

## Declaring default configuration

```go
func DefaultSpec() Spec {
    return Spec{Freq: 1 * timing.GHz, WallDistance: 10}
}
```

The `Comp` alias identifies the component's five element types. `DefaultSpec()`
returns a fresh starting configuration. A system builder can change its fields
and pass the result to `modeling.MakeBuilder[*Comp](spec)`.

Keep the function body to a single `return Spec{...}` with keyed fields and
constant leaves, because tooling reads the returned literal without running the
package. There is no separate `Definition` declaration.

The previous page defines `(*Middlewares).Initialize(c *Comp)`, which assembles
behavior after wiring. The walker needs no State initializer: the zero State
already means position 0 and no steps. Components needing allocated tables or
nonzero state define `(*State).Initialize(c *Comp)` as well. The simulation calls
State first, then Middlewares, once per component. Neither method returns a value.

## Building the Component

```go
s := sim.MakeBuilder().Build()

walker := modeling.MakeBuilder[*Comp](DefaultSpec()).
    WithSimulation(s).
    WithResources(Resources{RNG: rand.New(rand.NewSource(1))}).
    Build("Walker")
```

`modeling.MakeBuilder[*Comp](DefaultSpec())` returns a builder for this component type that
uses the returned Spec, and `Build` returns a `*Comp`. Set the simulation
the instance belongs to, supply its Resources — here a random source seeded
with `1`, so the output is reproducible — and build with a unique name. The
walker keeps the default Spec, so there is no `WithSpec` call, and it has no
ports, so there are no port bindings.

:::warning Checkpoint limitation of this example

The seed makes a fresh run reproducible, but the `*rand.Rand` in Resources
is not saved in a component checkpoint. Rebuilding with seed 1 restarts its
sequence; it does not resume the saved position. This introductory walker
therefore does not support checkpoint/resume. For a checkpointable model,
keep serializable RNG state in State, or use a registered resource that
implements checkpoint save/load for the RNG. See
[Writing Checkpointable Code](../checkpointing.md).

:::

`Build` validates the Spec and State shapes and registers the configured
instance. It leaves State and Middlewares unset. After all components and
connections are built and bound, call `s.Initialize()` once. Initialization
validates wiring, freezes topology, and runs `State.Initialize` and `Middlewares.Initialize`.

:::info The builder pattern

`modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(…).WithResources(…).Build("Walker")` is
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
`modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(…).WithSpec(…).WithResources(…).Build(name)`.
The whole configuration goes in as one `Spec` through `WithSpec` (there is
no setter per Spec field), starting from `DefaultSpec()`. That is
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

if err := s.Run(); err != nil {
    panic(err)
}

if err := s.Terminate(); err != nil {
    panic(err)
}
```

`TickLater` schedules the first tick at the next cycle. Nothing wakes a
component with no ports, so without it the engine has no events to fire and
`Run` returns immediately. Once a tick fires, the middleware returns `true`
and the component schedules its next tick automatically — you only call
`TickLater` once to start it (and again later if the component went idle
and now has work to do).

## Pausing from Another Goroutine

To control a running simulation, run the engine in one goroutine and request a
pause from another. After initialization and `walker.TickLater()`, the following
can replace the synchronous `Run` and `Terminate` calls above. It uses `context`,
`fmt`, and `time` from the standard library.

```go
runDone := make(chan error, 1)
go func() { runDone <- s.Run() }()

// This controller runs outside the engine's handlers and hooks.
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := engine.RequestPause().Wait(ctx); err != nil {
    panic(err)
}

var now timing.VTimeInPicoSec
if err := engine.Inspect(ctx, func() error {
    now = engine.CurrentTime()
    return nil
}); err != nil {
    panic(err)
}
fmt.Printf("Paused at %d ps\n", now)

if err := engine.Continue(); err != nil {
    panic(err)
}
if err := <-runDone; err != nil {
    panic(err)
}
if err := s.Terminate(); err != nil {
    panic(err)
}
```

The pause is acknowledged after the current event and hooks finish, or after all
workers and hooks in the current parallel batch finish. A fast run may finish
before the request arrives; the pause then applies to the idle engine and
persists until `Continue`. It does not select a deterministic virtual time.

Handlers and hooks may call `RequestPause`, but **must not call `Wait`**:
acknowledgment needs that callback to finish first. Send the returned request to
an external controller if the pause originates in a callback. Cancelling a wait
stops waiting; it does not undo the pause request.

Use `Inspect` to copy state at an execution boundary. A pause does not grant
exclusive ownership of model state if another controller can call `Continue`.
`engine.State().IsPaused()` reports true for both `EnginePaused` and
`EngineResuming`, because a queued continue has not taken effect yet. Call
`Continue` outside handlers and hooks as well; it waits for dispatch permission,
not for an event to execute.

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
`DefaultSpec()`, set its `Freq` to `100 * timing.MHz`, and pass it
with `WithSpec` — and you will see 530 ns instead, with the step count
unchanged. That is the whole point of a discrete-event simulator: the
model's behaviour is defined by cycles, and the clock just dictates how
those cycles map to simulated time.

## Key Concepts

- **A component is five structs, a `Comp` alias, and `DefaultSpec()`.** Spec is
  configuration, State is runtime data, Resources are references to shared
  objects, Ports talk to other components, and Middlewares are the
  behaviour.
- **A ticking component runs its middlewares every cycle**, in field order,
  while any of them makes progress.
- **Spec and State are plain JSON-serializable structs.** That is what
  lets the framework save component State; mutable Resources need their own
  checkpoint support, as the RNG limitation above illustrates.
- **`type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]`**
  is the per-package alias that keeps the long generic out of the rest of
  the code. `modeling.None` fills the Resources slot when there are no
  shared objects.
- **The system builder builds instances** with
  `modeling.MakeBuilder[*Comp](DefaultSpec())…Build(name)`; the component's package only declares
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
