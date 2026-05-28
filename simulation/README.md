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
- A registry of all components, ports, connections, and shared-state resources.

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

`Entities` returns typed references from the simulation's common entity
registry. It is useful for checkpoint inventory and tooling without requiring
components, ports, connections, and resources to share one implementation
interface.

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
