# modeling — Generic Component Framework

Package `modeling` provides the application-level component framework for the
Akita simulation framework. It builds on the `timing` and `messaging` packages
to offer generic, type-safe components with a structured Spec / State / Resources
separation and a middleware pipeline.

## Key Concepts

Every modeled component is parameterized by three type arguments:

- **Spec (`S`)** — immutable configuration set at build time (e.g., cache size,
  number of banks). Must be a plain struct with scalar fields only: booleans,
  numbers, strings, and named types based on them (such as `timing.Freq` or an
  enum-like string type). No slices, arrays, maps, or nested structs.
- **State (`T`)** — mutable runtime data (e.g., queues, counters, in-flight
  tables). May contain nested structs, slices, and maps; must be
  JSON-serializable.
- **Resources (`R`)** — references to shared objects (e.g., backing storage).
  Use `modeling.None` when a component references no shared resources.

Validate values at runtime with `ValidateSpec(v)` and `ValidateState(v)`; every
`Build` runs both. Both reject pointers, interfaces, channels, and functions, and
two fields that share a JSON name (which `encoding/json` silently drops).
`ValidateSpec` additionally rejects slices, arrays, maps, and nested structs;
`ValidateState` allows them, with `string` or integer map keys.

A value that seems to need a container in the Spec usually belongs elsewhere:

- one value repeated per unit (the same register count for every SIMD) is a
  single scalar;
- a reference to an external object, or something derived only from one (the
  address mapper that routes to lower memory), is a Resources field. Resources
  are not checkpointed: the rebuild supplies them, and `Build` recomputes what
  it derives from them.

## Component Models

Components are moving to component models, each in its own sub-package. A
component defined by a model is five structs — Spec, State, Resources, Ports,
and Middlewares — plus a package-level `Definition`. In every model, a
middleware implements `Middleware`, `Handle(e timing.Event) bool`: the
component passes each event it receives to its middlewares in field order.
The models differ in which events arrive:

- **`modeling/ticking`** — a clocked component: it receives a `TickEvent` on
  every cycle while it makes progress, and any event it schedules for itself.
  Choose it for pipelines and anything that does a bounded amount of work per
  cycle. See the package doc for how to declare and build one; `mem/rob` is the
  example.
- **Unclocked** (planned) — a component that runs when a message arrives, a
  port frees, or an event it scheduled fires; today's `EventDrivenComponent`.

The models share the building blocks in this package: `Middleware` and
`OrderedMiddlewares`, `PortTable` (a Ports struct bound to its component),
`WriteCheckpoint` and `ReadCheckpoint`, and `MustBeCheckpointable`. The API
below is the one most components still use; its middlewares are `Ticker`s,
`Tick() bool`.

## Key Types

### ComponentDef[S]

A component's defaults and boundary ports live in one public declaration, a
package-level var named `Definition`:

```go
var Definition = modeling.ComponentDef[Spec]{
    Name:        "MyComponent",
    DefaultSpec: Spec{Size: 64},
    Ports:       []modeling.PortDef{{Name: "Top"}},
}
```

The literal must be statically evaluable (keyed fields, constant leaves, role
identifiers) because the `inspect` package reads and validates it without
running the code. Treat it as read-only so builders and the inspector see the
same defaults and ports. Tooling finds the component's Resources type through
its builder's `WithResources` parameter.

- `Definition.DefaultSpec` is the starting configuration for a builder or
  caller to customize. Spec fields are scalars, so reading it yields an
  independent copy.
- `modeling.NewBuilder[...]().WithDefinition(Definition)` makes `Build` declare
  the definition's ports on the new component (the event-driven builder has
  the same method). A `PortDef` with `Group: true` declares a port group whose
  members are added at configuration time. Each component receives its own
  role slices.
- `Definition.Name`, `DefaultSpec`, and `Ports` expose the metadata directly.

### Component[S, T, R] (tick-driven)

A fixed-frequency component that processes state each tick via a middleware
pipeline.

```go
type MySpec struct {
    Size int
}

type MyState struct {
    Count int
}

comp := modeling.NewBuilder[MySpec, MyState, modeling.None]().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    WithSpec(MySpec{Size: 64}).
    Build("MyComponent")

comp.AddMiddleware(&myMiddleware{comp: comp})
```

- `Spec() S` — read-only accessor for the immutable spec (returns a copy).
- `Resources() R` — read-only accessor for the shared-resource references.
- `State` — a plain exported field of type `T`; middleware mutates it in place.
- `Tick() bool` — runs the middleware pipeline (returns true if progress made).

### EventDrivenComponent[S, T, R]

A component that wakes on events rather than ticking at a fixed frequency.

```go
comp := modeling.NewEventDrivenBuilder[MySpec, MyState, modeling.None]().
    WithSimulation(sim).
    WithSpec(MySpec{Size: 64}).
    WithProcessor(&myProcessor{}).
    Build("MyEDComponent")
```

The `EventProcessor[S, T, R]` interface has a single method:

```go
Process(comp *EventDrivenComponent[S, T, R], now timing.VTimeInPicoSec) bool
```

Wakeups are scheduled via:

- `comp.ScheduleWakeAt(t)` — schedule at a specific time (with dedup guard).
- `comp.ScheduleWakeNow()` — schedule at the current engine time.

Port notifications (`NotifyRecv`, `NotifyPortFree`) automatically schedule
wakeups.

### Domain

A named bundle of components that exposes selected internal ports at its
boundary. Domains nest — components form a domain (e.g., a shader array),
and domains compose into larger domains (e.g., a GPU) — with hierarchical
names following the `Domain.Domain.Component` convention.

```go
gpu := modeling.NewDomain("GPU[0]")
gpu.DeclarePort("Top")
gpu.AssignPort("Top", commandProcessor.GetPortByName("ToDriver"))

// Outside code addresses the domain, not its internals.
port := gpu.GetPortByName("Top")
```

## Builder Pattern

| Builder | Creates | Key Settings |
|---|---|---|
| `NewBuilder[S, T, R]()` | `*Component[S, T, R]` | `WithSimulation`, `WithFreq`, `WithSpec`, `WithResources` |
| `NewEventDrivenBuilder[S, T, R]()` | `*EventDrivenComponent[S, T, R]` | `WithSimulation`, `WithSpec`, `WithResources`, `WithProcessor` |

Both builders register the component as an event handler when the engine
implements `timing.HandlerRegistry`.

## Simulation context

All builders accept `timing.Simulation`, which supplies the engine and IDs
and registers components, connections, resources, and ports. Pass the same
simulation to every builder in one setup. Components expose
`Simulation()`; allocate a message or event ID with `comp.Simulation().NewID()`.
The simulation owns the counter, so two components share one ID sequence even
though they have different names. Independent simulations have separate sequences.

For isolated tests or small examples, create a lightweight context once:

```go
engine := timing.NewSerialEngine()
sim := modeling.NewStandaloneSimulation(engine)
```

Pass `sim` to each builder through `WithSimulation(sim)`. This context performs
no entity registration or recording. Use `simulation.MakeBuilder().Build()`
when automatic checkpoint inventory, recording, or monitoring is needed.
