# simulation

Package `simulation` provides the top-level simulation runner that wires
together an engine, data recorder, visual tracer, and optional monitoring
server. It also acts as a global state manager that registers every runtime
object as a named entity reachable through `GetStateByName`.

## Simulation

A `Simulation` holds:

- An `Engine` (serial or parallel) for event processing.
- A `DataRecorder` (SQLite-backed) for recording simulation data.
- A `DBTracer` for visual tracing (used by the Daisen visualizer).
- An optional `Monitor` for live web-based inspection.
- A global state manager: one flat entity inventory of all components, ports,
  connections, shared-state resources, and the engine and ID generator
  singletons, resolvable by name via `GetStateByName`.

The registry uses local minimal interfaces for components, ports, connections,
and resources. Concrete messaging types satisfy those interfaces structurally,
so this package does not need to import the messaging package.

### Building a Simulation

```go
sim := simulation.MakeBuilder().
    WithParallelEngine().       // optional: use parallel engine
    WithMonitorPort(8080).      // optional: monitoring server port
    WithVisTracingOnStart().    // optional: start tracing immediately
    Build()

defer sim.Terminate()
```

Builder options:

| Method | Description |
|--------|-------------|
| `WithParallelEngine()` | Use `ParallelEngine` instead of `SerialEngine` |
| `WithoutMonitoring()` | Disable the monitoring web server |
| `WithMonitorPort(port)` | Set the monitoring server port |
| `WithOutputFileName(name)` | Custom SQLite output file name |
| `WithVisTracingOnStart()` | Enable visual tracing from time 0 |

### Registering Components

```go
sim.RegisterComponent(myComponent)
```

Registration automatically:
- Adds the component and its ports to the simulation registry.
- Connects the component to the monitoring system (if enabled).
- Attaches visual tracing hooks.
- Registers any shared-state resources exposed by the component.

### Accessing Simulation Resources

```go
engine := sim.GetEngine()
recorder := sim.GetDataRecorder()
tracer := sim.GetVisTracer()
comp := sim.GetComponentByName("myComp")
port := sim.GetPortByName("myComp.Top")
resources := sim.Resources()
entities := sim.Entities()
```

### Global State Manager

The simulation is a global state manager: every registered runtime object —
component, port, connection, shared-state resource, and the engine and ID
generator singletons — is recorded as an `Entity` in one flat inventory.
`Entities()` returns these descriptors in registration order, which is useful
for inventory, debugging, and tooling without requiring every kind to share one
implementation interface.

Entity names are **globally unique across all kinds**, so any registered object
can be resolved by name. `GetStateByName` returns a live reference to the
entity's state, which the caller type-asserts:

```go
obj, ok := sim.GetStateByName("GPU[1].PageTable") // live state ref, or (nil, false)
pageTable := obj.(*vm.PageTable)                  // caller type-asserts
```

This is a deliberate access backdoor (similar to Unity's `GetComponent`): a
"magic" component can reach designed shared state — a page table, memory, or
allocator resource — directly by name and mutate it in place. The required type
assertion is the intentional warning that you are reaching past the normal
interfaces. Resolve the reference once at setup and cache it; `GetStateByName`
is a map lookup, not a free dereference, so it does not belong on a hot path.
The reserved singleton names `engine` and `id-generator` must not be reused by
user objects.

### How an entity exposes its state

By default `GetStateByName` returns the registered entity value itself. An
entity whose serializable state lives in a distinct sub-object implements
`StateHolder` to expose a live reference to it:

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Component` | `Name() string` | Register runtime components |
| `Port` | `Name() string` | Register port buffers by stable name |
| `Connection` | `Name() string` | Register runtime connections |
| `ResourceOwner` | `Resources() []Resource` | Register shared program state |
| `Resource` | `Name()/Kind()/Identity()` | Shared state reachable by name |
| `StateHolder` | `StateRef() State` | Expose a live reference to runtime state |

`modeling.Component[S,T]` and `modeling.EventDrivenComponent[S,T]` implement
`StateHolder`, returning a pointer to their `State` field.

`modeling.Component[S,T]` and `modeling.EventDrivenComponent[S,T]` satisfy
these interfaces automatically.
