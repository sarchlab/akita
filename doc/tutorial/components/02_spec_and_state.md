---
sidebar_position: 2
---

# Spec, State, and Resources

Every component starts with plain Go structs: an immutable **Spec**, a
mutable **State**, the **Resources** it references, and its **Ports**. For
the random walk they are tiny.

## Spec — Configuration

```go
type Spec struct {
    Freq         timing.Freq `json:"freq"`
    WallDistance int         `json:"wall_distance"`
}
```

`Spec` is set once when the component is built and never changes at
runtime. `Freq` is the clock the component ticks at; every ticking
component's Spec has a `Freq timing.Freq` field, and `Build` panics without
one. `WallDistance` is our own setting — "stop when the walker drifts 10
units from the origin in either direction". By convention the **clock
frequency lives in the Spec** alongside the rest of a component's
configuration (in larger components: buffer sizes, latencies, thresholds),
so the full configuration travels — and serializes — together.

## State — Runtime Data

```go
type State struct {
    Position int `json:"position"`
    Steps    int `json:"steps"`
}
```

`State` is what changes during the run: where the walker is, and how
many steps it has taken. State holds everything the component must
remember between cycles, and only the component's own code writes it.

Both structs carry JSON tags on every field and hold only plain data. That
is what makes an Akita simulation checkpoint-friendly without any extra work
from you: the Spec identifies the configuration, and the State is what a
checkpoint saves and restores. The two differ in what they may hold:

| | Allowed field types |
|---|---|
| **Spec** | Scalars (`bool`, integers, floats, `string`, and named types based on them, such as `timing.Freq` or an enum-like `type Mode string`) and slices or arrays of scalars. |
| **State** | Scalars, plus slices, arrays, maps with string or integer keys, and nested structs. |

Neither may hold pointers, interfaces, channels, or functions. A reference to
an external object, such as a backing storage, goes in the component's
Resources, described next.

The Spec is flat on purpose. It is the configuration users edit and tools
display, so every field is one setting with one default. A setting may be a
list of scalars, such as the remote ports a unit talks to, whose length
depends on the system. Other values that seem to belong in the Spec usually
belong elsewhere:

- **One value repeated per unit**, such as the same register count for every
  SIMD: use a single scalar.
- **A reference to another object**, or something derived only from one,
  such as the address mapper that picks which lower memory serves an
  address: put it in Resources. Resources are not checkpointed; the system
  builder that rebuilds the simulation supplies them again, and the
  component's `NewMiddlewares` recomputes anything derived from them.

`Build` checks these rules and panics if the Spec has a map, nested struct,
pointer, or interface field, or a slice or array of anything but scalars, or
if the State cannot be checkpointed.

## Resources — Shared Objects

```go
type Resources struct {
    RNG *rand.Rand
}
```

`Resources` holds references to objects the component uses but does not
own; the system builder supplies them when it builds the instance. The
walker draws its steps from a random source, and making that source a
Resource means the system builder, not the component, chooses the seed.
Because Resources are references rather than data, they may hold pointers
and interfaces.

A component that references no shared objects puts `modeling.None` in the
Resources slot instead of declaring an empty struct — the components in the
next section do.

## Ports — None Yet

```go
type Ports struct{}
```

The walker talks to no one, so its `Ports` struct is empty. The next
section gives a component its first port.

## The `Comp` Alias

Now that the structs exist, define the component alias next to them:

```go
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
```

`ticking.Component` is generic over the five structs. The fifth,
`Middlewares`, holds the behaviour and is the subject of the next page. The
rest of the example refers to `Comp` and `*Comp` instead of repeating the
full generic.

## Where to Next

The structs hold data but do nothing. The behaviour lives in
**middlewares** — the next page.
