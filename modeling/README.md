# modeling — Component Framework

Package `modeling` provides the component framework of the Akita simulation
framework. It builds on the `timing` and `messaging` packages. A component is
defined by five structs and one of three component models, each a
sub-package: `modeling/ticking`, `modeling/wakeup`, and `modeling/event`. This
package holds what the models share.

## Five Structs

A component type is five structs, all declared in its package, plus a
package-level `Definition`:

| Struct | What it is | Supplied by |
|---|---|---|
| `Spec` | configuration: scalars and slices of scalars | system builder (defaults in `Definition`) |
| `State` | mutable runtime data, saved in checkpoints | component (`NewState`) |
| `Resources` | references to shared objects | system builder |
| `Ports` | one `messaging.Port` field per port, `[]messaging.Port` per group | system builder (`messaging.NewPort`) |
| `Middlewares` | the behavior, one field per middleware | component (`NewMiddlewares`) |

The system builder is the code that assembles a simulation. It builds every
instance with `Definition.Builder()...Build(name)`, passing all of its ports;
`Build` binds and registers them, and no port is added later. Use
`modeling.None` for the Resources of a component that references no shared
objects.

- **Spec** must be a plain struct whose fields are scalars (booleans, numbers,
  strings, and named types based on them, such as `timing.Freq`, a
  `messaging.RemotePort`, or an enum-like string type) or slices of scalars,
  such as the list of remote ports a unit talks to, whose length depends on the
  system. No maps or nested structs. `Build` copies the slices, so an instance
  never shares one with `DefaultSpec`; treat the slices `Spec()` returns as
  read-only.
- **State** may contain nested structs, slices, and maps; it must be
  JSON-serializable. Only the component writes it: its `NewState`, which may
  read the Spec, Resources, and Ports, and its middlewares. A function in the
  component's own package may also write it while no event is running, for
  example to queue work before the simulation starts (`ping.SchedulePing`);
  nothing writes it while events run, so the parallel engine needs no locks.
- **Middlewares** hold only references: the component, other middlewares, and
  immutable values derived from the Spec and Resources. All mutable data lives
  in State, because only State is saved in checkpoints.

`ValidateSpec(v)` and `ValidateState(v)` check the rules at run time, and every
`Build` runs both through `MustBeCheckpointable`. Both reject pointers,
interfaces, channels, and functions, and two fields that share a JSON name
(which `encoding/json` silently drops). `ValidateSpec` additionally rejects
maps, nested structs, and slices of anything but scalars; `ValidateState`
allows them, with `string` or integer map keys.

A Spec may hold a slice of scalars, such as the remote ports a unit talks to.
Other values that seem to belong in the Spec usually belong elsewhere:

- one value repeated per unit (the same register count for every SIMD) is a
  single scalar;
- a value derived from other Spec fields is an unexported Spec method, not a
  field;
- a reference to an external object, or something derived only from one (the
  address mapper that routes to lower memory), is a Resources field. A
  component's Resources are not part of its checkpoint: the rebuild supplies
  them, and `NewMiddlewares` recomputes what it derives from them. A shared
  object they point to, such as a `mem.Storage` or a `vm.PageTable`, is a
  registered resource that checkpoints itself.

## Component Models

In every model a middleware is an event handler, `Middleware.Handle(e
timing.Event) bool`: the component passes each event it receives to its
middlewares in field order, and a middleware ignores the events it does not
handle. The models differ only in which events arrive:

| Model | Events that arrive | Example |
|---|---|---|
| `modeling/ticking` | a `ticking.TickEvent` every cycle while any middleware makes progress; port activity restarts ticking | `mem/rob` |
| `modeling/wakeup` | a data-less `wakeup.Event` on port activity or at a time the component asked for (`WakeAt`); it runs again at once while any middleware makes progress | `examples/ping` |
| `modeling/event` | `event.Recv` and `event.PortFree` for port activity, and the events, with their data, that the component schedules for itself | the `modeling/event` example |

Choosing a model:

- **ticking** — the behavior advances cycle by cycle and does a bounded
  amount of work per cycle: a pipeline, a cache, a TLB, a switch. It is the
  default. Cost: an event per active cycle.
- **wakeup** — the component is idle most of the time and knows when it next
  has work, and its middlewares poll the State and ports whenever it runs. An
  idle component costs nothing, but it must ask for every future wakeup.
- **event** — the behavior is a set of distinct happenings, each with its own
  data and time: a request that completes after a computed latency, a
  traffic generator's timed actions. Each middleware reacts to specific event
  types instead of polling.

Each package doc shows how to declare and build a component of its model.

## Shared Building Blocks

- `Component` — the interface every model's `Component` implements: named,
  hookable, the handler of its events, and the owner of its ports
  (`messaging.PortOwner`).
- `Middleware` — one piece of a component's behavior, `Handle(e) bool`.
- `WriteCheckpoint` and `ReadCheckpoint` — save and restore a component's
  Spec hash, State, and the dedup guard of its scheduler. A `Scheduler` is
  what they save the guard through. The ticking and wakeup models each keep
  their own scheduler next to the event it schedules; `ticking.Scheduler` is
  exported for connections written by hand, such as `directconnection`.
- `modelingtest` — `CheckTicking`, `CheckWakeup`, and `CheckEvent` assert that
  the inspector's static view of a package matches its `Definition`;
  `Tick` steps a ticking component by one cycle in tests.
- Every model's `Component` has the same methods — `Name`, `TypeName`,
  `NewID`, `CurrentTime`, `Spec`, and `Resources` — and the `State`, `Ports`,
  and `Middlewares` fields. They come from a base type in
  `modeling/internal/base`, which also holds the steps of `Build`: a
  component model is defined only by the three model packages.

### Domain

A named bundle of components that exposes selected internal ports at its
boundary. Domains nest — components form a domain (e.g., a shader array),
and domains compose into larger domains (e.g., a GPU) — with hierarchical
names following the `Domain.Domain.Component` convention.

The boundary is a struct of ports, like a component's `Ports`; each field holds
the internal port it exposes.

```go
type GPUPorts struct {
    Top messaging.Port
}

gpu := modeling.NewDomain("GPU[0]", GPUPorts{Top: commandProcessor.Ports.ToDriver})

// Outside code addresses the domain, not its internals.
port := gpu.Ports.Top
```

## Simulation context

Every builder takes a `timing.Simulation`, which supplies the engine and IDs
and registers components, connections, resources, and ports. Pass the same
simulation to every builder in one setup. A component keeps its simulation
to itself; allocate a message or event ID with `comp.NewID()`. The simulation
owns the counter, so two components share one ID sequence even though they
have different names. Independent simulations have separate
sequences.

For isolated tests or small examples, create a lightweight context once:

```go
engine := timing.NewSerialEngine()
sim := modeling.NewStandaloneSimulation(engine)
```

Pass `sim` to each builder through `WithSimulation(sim)`. This context performs
no entity registration or recording. Use `simulation.MakeBuilder().Build()`
when automatic checkpoint inventory, recording, or monitoring is needed.
