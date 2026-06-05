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
  connections, and shared-state resources, resolvable by name via
  `GetStateByName`.

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
component, port, connection, and shared-state resource — satisfies the `Entity`
interface and is held in one flat inventory. `Entities()` returns those entities
in registration order, which is useful for inventory, debugging, and tooling.

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

## Checkpoint and Resume

A simulation can be checkpointed to a `.tar.gz` archive and resumed later — for
example, to snapshot between GPU kernels or to restart a long run. The contract
is an oracle: *running to the end* must equal *checkpoint, rebuild, restore, run
to the end*.

```go
// Save (engine stopped, outside an event handler):
err := sim.SaveCheckpoint("snap.tar.gz", "")  // "" uses the default build ID

// Resume: rebuild the *identical* simulation with the same setup code, then:
err := sim.LoadCheckpoint("snap.tar.gz", "")
engine.Run()                                   // continue to completion
```

Setup rebuilds the *shape* (components, ports, connections, resources, wiring);
the checkpoint restores only the *runtime* (each entity's `State`, port buffers,
shared resources, the event queue, the engine time, and the ID-generator
counter). The archive holds a `build_id` entry plus one payload per entity; the
payload files are the inventory (there is no manifest). Loading validates the
build ID and that the saved and rebuilt entity sets match exactly.

### The golden rule: all mutable runtime state lives in `State`

Anything that changes during simulation and is not derivable from the restored
queue **must** be a field of the component's `State` (or a registered resource).
Runtime state hidden on a middleware struct — a round-robin cursor, a counter, an
RNG — is *not* checkpointed, so a resumed run silently diverges. Keep middleware
fields to structural wiring (ports, downstream references, routing tables) that
setup rebuilds; put cursors and counters in `State`.

### Requirements and tips

- **Serial engine only.** `SaveCheckpoint`/`LoadCheckpoint` reject a
  `ParallelEngine`.
- **Run with tracing off** for a deterministic resume: the tracing task-ID side
  table consumes the global ID generator, perturbing the ID sequence.
- **`SerialEngine.RunUntil(t)`** stops the engine at a deterministic boundary
  (every event with time ≤ `t`), unlike `Run` (drains everything) or `Pause`
  (stops at a non-reproducible point) — useful for taking a mid-transaction
  checkpoint.
- An entity package becomes checkpointable by implementing the structural
  `Checkpointable` interface (`SaveCheckpoint(io.Writer)` / `LoadCheckpoint(io.Reader)`);
  it never imports `simulation`. `modeling.Component`/`EventDrivenComponent`,
  ports, `mem.Storage`, and `vm.PageTable` already do.

See `examples/checkpointdemo` for a runnable save/load demo and
`mem/acceptancetests/checkpointresume` for a mid-transaction resume oracle.

### Entity interfaces

`Entity` is the abstract base interface; each kind embeds it and adds its own
capabilities:

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `Entity` | `Name() string` | Abstract base for every registered object |
| `Component` | `Entity` | Register runtime components |
| `Port` | `Entity` + `NumIncoming()/NumOutgoing()` | Register port buffers |
| `Connection` | `Entity` | Register runtime connections |
| `Resource` | `Entity` | Shared state registered by setup, reachable by name |

`GetStateByName` returns the registered entity itself — a component, port,
connection, or resource — which the caller type-asserts. Shared resources are
owned by the simulation and registered by setup via `RegisterResource`;
components hold references to them rather than embedding the payload.
