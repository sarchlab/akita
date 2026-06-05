---
sidebar_position: 4
---

# Building and Running

With the Spec, State, and middleware defined, `main` assembles the
component and runs the engine.

## Building the Component

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

`modeling.NewBuilder` is the generic constructor for a
`Component[Spec, State, Resources]`, and it returns a `*Comp`. Note that the
type arguments are still spelled out here even though we defined `Comp`:
`NewBuilder` is a generic *function* with its own `[S, T, R]` parameters, and
an alias for the component *type* cannot be substituted for them. The alias
still earns its keep on the middleware field and this return value — just not
on the constructor call. (The per-package builders in the next section hide
the generics by writing them once, inside `Build`.) Set the engine, the clock
frequency, and the spec, then build with a unique name.

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
