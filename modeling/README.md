# modeling

Package `modeling` provides the application-level component framework for Akita
simulations. It builds on `sim` to offer generic, type-safe components with
structured Spec+State separation and middleware pipelines.

## Key Concepts

### Spec and State

Every modeled component is parameterized by two type arguments:

- **Spec (`S`)** — immutable configuration set at build time (e.g., cache size,
  number of banks). Must be a plain struct with primitive fields only.
- **State (`T`)** — mutable runtime data (e.g., queues, counters, in-flight
  tables). May contain nested structs, slices, and maps. Must be
  JSON-serializable.

Use `ValidateSpec(v)` and `ValidateState(v)` at runtime to verify compliance.
No pointers, interfaces, channels, or functions are allowed in either.

## Component Types

### Component[S, T] (tick-driven)

A fixed-frequency component that processes state each tick via a middleware
pipeline.

```go
type MySpec struct {
    Size int
}

type MyState struct {
    Count int
}

builder := modeling.NewBuilder[MySpec, MyState]().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithSpec(MySpec{Size: 64})

comp := builder.Build("MyComponent")
comp.AddMiddleware(&myMiddleware{comp: comp})
```

Key methods:

- `Spec S` — returns the immutable spec.
- `State T` / ``State` field *T` — read/write the current state.
- `Tick() bool` — runs the middleware pipeline (returns true if progress made).

### EventDrivenComponent[S, T]

A component that wakes on events rather than ticking at a fixed frequency.

```go
builder := modeling.NewEventDrivenBuilder[MySpec, MyState]().
    WithEngine(engine).
    WithSpec(MySpec{Size: 64}).
    WithProcessor(&myProcessor{})

comp := builder.Build("MyEDComponent")
```

The `EventProcessor[S, T]` interface has a single method:

```go
Process(comp *EventDrivenComponent[S, T], now sim.VTimeInPicoSec) bool
```

Wakeups are scheduled via:

- `comp.ScheduleWakeAt(t)` — schedule at a specific time (with dedup guard).
- `comp.ScheduleWakeNow()` — schedule at the current engine time.

Port notifications (`NotifyRecv`, `NotifyPortFree`) automatically schedule
wakeups.

## Builders

| Builder | Creates | Key Settings |
|---------|---------|-------------|
| `NewBuilder[S,T]()` | `*Component[S,T]` | `WithEngine`, `WithFreq`, `WithSpec` |
| `NewEventDrivenBuilder[S,T]()` | `*EventDrivenComponent[S,T]` | `WithEngine`, `WithSpec`, `WithProcessor` |

## Validation

```go
err := modeling.ValidateSpec(MySpec{Size: 64})  // checks no nested structs
err := modeling.ValidateState(MyState{})         // allows nested structs
```

Both reject pointers, interfaces, channels, and functions. Map keys must be
`string` or integer types.
