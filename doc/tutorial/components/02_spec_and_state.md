---
sidebar_position: 2
---

# Spec and State

Every component starts with two plain Go structs: an immutable **Spec**
and a mutable **State**. For the random walk they are tiny.

## Spec — Configuration

```go
type walkSpec struct {
    WallDistance int `json:"wall_distance"`
}
```

`walkSpec` is set once when the component is built — here, "stop when the
walker drifts 10 units from the origin in either direction". Spec holds
the things that never change at runtime: in larger components this is
clock frequency, port buffer sizes, latencies, thresholds.

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

Both structs carry JSON tags. That is the rule for Spec and State: use
primitive, JSON-serializable types and tag every field. It is what makes
an Akita simulation checkpoint-friendly without any extra work from you —
the engine can serialize the whole simulation by serializing each
component's Spec and State.

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
