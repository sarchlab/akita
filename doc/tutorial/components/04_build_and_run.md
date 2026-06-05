---
sidebar_position: 4
---

# Building and Running

With the Spec, State, and middleware defined, `main` assembles the
component and runs the engine.

## Building the Component

```go
s := simulation.MakeBuilder().Build()

spec := walkSpec{Freq: 1 * timing.GHz, WallDistance: 10}
comp := modeling.NewBuilder[walkSpec, walkState, modeling.None]().
    WithEngine(s.GetEngine()).
    WithFreq(spec.Freq).
    WithSpec(spec).
    Build("Walker")
comp.AddMiddleware(&walkMW{
    comp: comp,
    rng:  rand.New(rand.NewSource(1)),
})
```

`modeling.NewBuilder` is the generic constructor for a
`Component[Spec, State, Resources]`, and it returns a `*Comp`. Note that the
type arguments are still spelled out here even though we defined `Comp`:
`NewBuilder` is a generic *function* with its own `[S, T, R]` parameters, and
an alias for the component *type* cannot be substituted for them. The alias
still earns its keep on the middleware field and this return value — just not
on the constructor call. (The per-package builders in the next section hide
the generics by writing them once, inside `Build`.) Set the engine, the clock
frequency (read from the spec), and the spec itself, then build with a unique
name.

:::info The builder pattern

`NewBuilder().WithEngine(…).WithFreq(…).WithSpec(…).Build("Walker")` is the
**builder pattern**: rather than one constructor with a long argument list,
you assemble the component through a chain of small `WithX` calls and finish
with `Build`. It reads top to bottom, and each setting is independent.

**How the chaining works.** Every `WithX` takes the builder *by value*, sets
one field on its copy, and returns that copy. Because each call returns a
builder, the next one hangs off it. Nothing is mutated in place, so a
partially configured builder is just a value you can stash and reuse.

**Akita's convention.** Per-package builders — the subject of the next
section — take a component's whole configuration as one `Spec` through
`WithSpec` (there is no setter per spec field) and start from a
`DefaultSpec()`. That is why the clock frequency lives in the spec. They also
follow a fixed shape, `MakeBuilder().WithRegistrar(…).WithSpec(…).Build(name)`,
and create their ports inside `Build`. The low-level `modeling.NewBuilder`
shown here cannot pull `Freq` out of an arbitrary spec type on its own, so we
hand it over explicitly with `WithFreq(spec.Freq)`; a per-package builder does
that once, inside `Build`.

:::

`AddMiddleware` registers the middleware that will run every cycle. The RNG
is seeded with `1` so the output is reproducible.

## Kicking It Off

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
zero — that is one realisation of a random walk. With seed 1 the trajectory
is fixed, so every run prints exactly this line. Change the seed and the
path (and the step count) changes.

The simulated time, 53000 ps = 53 ns, equals 53 cycles at 1 GHz — that is
the 52 walking steps plus one for the `TickLater` that started the clock.
Try setting the frequency to 100 MHz and you will see 530 ns instead, with
the step count unchanged. That is the whole point of a discrete-event
simulator: the model's behaviour is defined by cycles, and the clock just
dictates how those cycles map to simulated time.

## Key Concepts

- **A component runs its middleware every cycle.** Spec is configuration,
  State is runtime data, middleware is the per-cycle work.
- **Both Spec and State are plain JSON-serializable structs.** That is what
  makes Akita simulations checkpoint-friendly out of the box.
- **`type Comp = modeling.Component[Spec, State, modeling.None]`** is the
  per-package alias that keeps the long generic out of the rest of the
  code. `modeling.None` is the sentinel for "no shared resources".
- **Progress drives the loop.** Return `true` from `Tick` to be called
  again next cycle; return `false` to stop. Outside events (`TickLater`,
  incoming messages) can wake an idle component back up.
- **Middleware is just a Go struct.** It can carry whatever auxiliary data
  the behaviour needs (here, a random source).

## Where to Next

This component talks to nobody. The next section, **Make Components Talk to
Each Other**, introduces **ports** for sending and receiving messages,
**multiple middlewares** that share one component's state, and the
per-package **builder** convention used throughout Akita.
