---
slug: /simulation
---

# sim — Simulation Framework and Runner

Package `sim` provides the top-level simulation runner for the Akita
simulation framework. It wires together an engine, a data recorder, a visual
tracer, and an optional monitoring server, and acts as a global inventory that
registers every runtime object as a named entity.

## Package layout

The `sim` package assembles and runs a simulation. Its subpackages
provide independently usable building blocks:

| Package | Responsibility |
|---------|----------------|
| `sim/naming` | Hierarchical names |
| `sim/hooking` | Observation hooks |
| `sim/queueing` | Buffers and pipelines |
| `sim/timing` | Time, events, and engines |
| `sim/messaging` | Messages, ports, and protocols |
| `sim/modeling` | Component definitions and execution models |
| `sim/tracing` | Trace events and collectors |
| `sim/datarecording` | Recording simulation results |
| `sim/sourcefs` | Archiving and reading recorded source |

These packages remain in the same Go module. Import, for example,
`github.com/sarchlab/akita/v5/sim/modeling/ticking`. Runtime subpackages
use the `timing.Simulation` interface rather than importing the parent runner.
Hardware models (`mem`, `noc`) and tools (`monitoring`, `daisen`, `inspect`)
remain outside this directory. The runner has no dependency on either UI.

## How It Works

A `Simulation` is created through its `Builder`, which constructs and connects
the supporting infrastructure. Once built, you register components, connections,
and shared resources with the simulation; registration tracks each object in a
flat inventory and wires it into tracing and monitoring. The simulation exposes
typed accessors for the engine and infrastructure, plus name-based and
inventory accessors for the registered entities. After all ports and connections
are bound, call `s.Initialize()` to validate wiring and create component State
and Middlewares. This freezes topology; seed work and run only afterward.

The runner depends only on minimal local interfaces (`Component`, `Port`,
`Connection`, `Resource`). Concrete messaging types satisfy them structurally,
so this package does not import the messaging package.

## Key Concepts

- A `Simulation` holds an `Engine` (serial or parallel), a SQLite-backed
  `DataRecorder`, a `DBTracer` for visual tracing (used by the Daisen
  visualizer), and an optional `Monitor` for live web inspection.
- Every registered runtime object — component, port, connection, and
  shared-state resource — satisfies the `Entity` interface and is held in one
  flat inventory. Entity names are **globally unique across all kinds**.
- A `Resource` is non-timing program state (such as memory contents or page
  tables) that multiple components reference. The simulation owns resources;
  components hold references to them rather than embedding the payload.

## Entity Interfaces

`Entity` is the abstract base interface; each kind embeds it and adds its own
capabilities:

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `Entity` | `Name() string` | Abstract base for every registered object |
| `Component` | `Entity` | Runtime components |
| `Port` | `Entity` + `NumIncoming()` / `NumOutgoing()` | Port buffers |
| `Connection` | `Entity` | Runtime connections |
| `Resource` | `Entity` | Shared state registered by setup, reachable by name |

## Builder Pattern

```go
monitor := monitoring.NewMonitor().WithPortNumber(8080)
s := sim.MakeBuilder().
    WithParallelEngine().       // optional: use the parallel engine
    WithMonitor(monitor).       // optional: enable live monitoring
    WithVisTracingOnStart().    // optional: start tracing immediately
    Build()
defer s.Terminate()
```

| Method | Description |
|--------|-------------|
| `WithParallelEngine()` | Use `ParallelEngine` instead of `SerialEngine` |
| `WithMonitor(monitor)` | Attach an optional monitor; omitted by default |
| `WithOutputFileName(name)` | Custom SQLite output file name |
| `WithVisTracingOnStart()` | Enable visual tracing from time 0 |

### Optional monitoring

`MakeBuilder().Build()` starts no monitoring server. To enable the web UI,
import `github.com/sarchlab/akita/v5/monitoring` and supply a monitor as shown
above. After creating the engine, tracer, and recorder, the runner calls
`Start(s)` to bind and start the monitor. It forwards component and port
registrations and stops the monitor before closing those resources in `Terminate`.
Each monitor belongs to one simulation and must not be reused.

The visualization tracer is always constructed; tracing is enabled separately.
Monitors can access runtime services through `Engine()`, `DataRecorder()`,
`VisTracer()`, and `TraceDBPath()` without changing their lifecycle interface.

A custom monitor implements `sim.Monitor` with `Start(*sim.Simulation)`,
`RegisterComponent`, `RegisterPort`, and `Stop`. Keep the concrete monitor in
application setup for implementation-specific features such as
`monitor.CreateProgressBar`; `s.Monitor()` exposes only the runtime contract.
The `sim` runtime has no dependency on monitoring, Daisen, or component libraries;
only the root `sim` package knows about the monitor interface.

## Usage

### Registering Components

```go
s.RegisterComponent(myComponent)
```

Registration adds the component to the inventory, attaches the visual
tracer, and connects the component to the monitor (if enabled). A component's
`Build` registers the component, and `BindPort` registers each port
(`RegisterPort`). A system builder calls `RegisterComponent` only for a
component written without a component model. Use `RegisterConnection` and
`RegisterResource` to register connections and shared resources.

### Accessing the Simulation

```go
engine := s.Engine()
recorder := s.DataRecorder() // a simulator can write its own tables into the recording
monitor := s.Monitor()       // nil when monitoring is off
```

## Checkpoint and Resume

A simulation can be checkpointed to a `.tar.gz` archive and resumed later — for
example, to snapshot between GPU kernels or to restart a long run. The contract
is an oracle: *running to the end* must equal *checkpoint, rebuild, restore, run
to the end*.

```go
// Save (engine stopped, outside an event handler):
err := s.SaveCheckpoint("snap.tar.gz", "")  // "" uses the default build ID

// Resume: rebuild the *identical* simulation, bind its links, and successfully
// call s.Initialize() with the same setup code, then:
err := s.LoadCheckpoint("snap.tar.gz", "")
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
  table is not checkpointed; rebuilding associations consumes the simulation's
  ID counter, perturbing the resumed ID sequence.
- **`SerialEngine.RunUntil(t)`** stops the engine at a deterministic boundary
  (every event with time ≤ `t`), unlike `Run` (drains everything) or `Pause`
  (stops at a non-reproducible point) — useful for taking a mid-transaction
  checkpoint.
- **Register your message and event types.** A port holds `messaging.Msg` values whose non-nil payloads must be registered
  at package initialization
  and the engine queue any `timing.Event`; each concrete type must be registered
  (a message by listing it in a `messaging.DefineProtocol`, an event with
  `timing.RegisterEvent` in an `init()`) so a checkpoint that captures it can
  be decoded. A forgotten registration fails
  loudly at load.
- An entity package becomes checkpointable by implementing the structural
  `Checkpointable` interface (`SaveCheckpoint(io.Writer)` / `LoadCheckpoint(io.Reader)`);
  it never imports `sim`. components built with the component models,
  ports, `mem.Storage`, and `vm.PageTable` already do.

**Writing your own checkpointable messages, events, and components:** see
[`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md) for the full checklists, a
worked example, and the gotchas.

See `examples/checkpointdemo` for a runnable save/load demo and
`mem/acceptancetests/checkpointresume` for a mid-transaction resume oracle.
