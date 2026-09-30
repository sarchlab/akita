---
sidebar_position: 2
---

# Spec and State

Every component starts with two plain Go structs: an immutable **Spec**
and a mutable **State**. For the random walk they are tiny.

## Spec — Configuration

```go
type walkSpec struct {
    Freq         timing.Freq `json:"freq"`
    WallDistance int         `json:"wall_distance"`
}
```

`walkSpec` is set once when the component is built and never changes at
runtime. `Freq` is the clock the component ticks at; `WallDistance` is our
own setting — "stop when the walker drifts 10 units from the origin in either
direction". By convention the **clock frequency lives in the spec** alongside
the rest of a component's configuration (in larger components: port buffer
sizes, latencies, thresholds), so the full configuration travels — and
serializes — together.

## State — Runtime Data

```go
type walkState struct {
    Position int `json:"position"`
    Steps    int `json:"steps"`
}
```

`walkState` is what changes during the run: where the walker is, and how
many steps it has taken. State holds everything the component must
remember between cycles.

Both structs carry JSON tags on every field and hold only plain data. That
is what makes an Akita simulation checkpoint-friendly without any extra work
from you: the engine can serialize the whole simulation by serializing each
component's Spec and State. The two differ in what they may hold:

| | Allowed field types |
|---|---|
| **Spec** | Scalars only: `bool`, integers, floats, `string`, and named types based on them (such as `timing.Freq` or an enum-like `type Mode string`). |
| **State** | Scalars, plus slices, arrays, maps with string or integer keys, and nested structs. |

Neither may hold pointers, interfaces, channels, or functions. A reference to
an external object, such as a backing storage, goes in the component's
Resources, the third type parameter introduced below.

The Spec is flat on purpose. It is the configuration users edit and tools
display, so every field is one setting with one default. When a setting
seems to need a list, it is usually one of these instead:

- **One value repeated per unit**, such as the same register count for every
  SIMD: use a single scalar.
- **Something the builder computes from the wiring**, such as the remote
  ports an address mapper routes to: compute it in `Build` and keep it in
  State.
- **A reference to another object**: put it in Resources.

`Build` checks these rules and panics if the Spec has a slice, array, map,
nested struct, pointer, or interface field, or if the State cannot be
checkpointed.

## The `Comp` Alias

Now that the Spec and State types exist, define the component alias next to
them:

```go
type Comp = modeling.Component[walkSpec, walkState, modeling.None]
```

`modeling.Component` is generic over Spec, State, and a (rarely used)
shared-resources type; `modeling.None` fills the resources slot with
"none". The rest of the example refers to `Comp` and `*Comp` instead of
repeating the full generic.

## Where to Next

The structs hold data but do nothing. The behaviour lives in
**middleware** — the next page.
