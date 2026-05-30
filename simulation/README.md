# simulation

Package `simulation` provides the top-level simulation runner that wires
together an engine, data recorder, visual tracer, and optional monitoring
server. It also provides checkpoint save/load support.

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
for checkpoint inventory and tooling without requiring every kind to share one
implementation interface.

Entity names are **globally unique across all kinds**, so any registered object
can be resolved by name:

```go
obj, ok := sim.GetStateByName("GPU[1].PageTable") // live object, or (nil, false)

// Typed form: resolves and type-asserts in one step.
pageTable, ok := simulation.GetState[*vm.PageTable](sim, "GPU[1].PageTable")
```

This is a deliberate access backdoor (similar to Unity's `GetComponent`): a
"magic" component can reach designed shared state — a page table, memory, or
allocator resource — directly by name. Resolve the handle once at setup and
cache it; `GetStateByName` is a map lookup, not a free dereference, so it does
not belong on a hot path. The reserved singleton names `engine` and
`id-generator` must not be reused by user objects.

## Checkpoint Save / Load

Save and load full simulation state to/from a directory:

```go
// Save (requires quiescence — all port buffers must be empty)
err := sim.Save("/path/to/checkpoint")

// Load (simulation must already be built with same topology)
err := sim.Load("/path/to/checkpoint")
```

### What Gets Saved

- **Metadata**: engine time, ID generator state.
- **Component states**: JSON files for each component implementing `StateSaver`.
- **Shared state resources**: payload files for registered resources, such as
  memory storage or page tables.

### What Is NOT Saved

- Event queues (reconstructed via `TickLater` / port notifications).
- Connections (reconstructed from build code).
- Port buffer contents (must be empty — quiescence required).

### Interfaces for Checkpointing

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Component` | `Name() string` | Register runtime components |
| `Port` | `Name() string` | Register port buffers by stable name |
| `Connection` | `Name() string` | Register runtime connections |
| `StateSaver` | `SaveState(w io.Writer)` | Serialize component state |
| `StateLoader` | `LoadState(r io.Reader)` | Deserialize component state |
| `ResourceOwner` | `Resources() []Resource` | Register shared program state |
| `Resource` | `Save(w io.Writer)` | Serialize shared program state |
| `WakeupResetter` | `ResetWakeup()` | Reset wakeup guard after load |

`modeling.Component[S,T]` and `modeling.EventDrivenComponent[S,T]` satisfy
these interfaces automatically.
