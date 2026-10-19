---
slug: /simulation
---

# sim — Simulation Framework and Runner

Package `sim` provides the top-level simulation runner for the Akita
simulation framework. It wires together an engine and optional recording, visual
tracing, and monitoring services, and acts as a global inventory that
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
| `sim/tasks` | Task lifecycles, identities, and observer subscriptions |
| `sim/tasks/tracing` | Built-in task observers for recording and statistics |
| `sim/datarecording` | Recording simulation results |
| `internal/sourcefs` | Archiving and reading recorded source |

These packages remain in the same Go module. Import, for example,
`github.com/sarchlab/akita/v5/sim/modeling/ticking`. Runtime subpackages
use the `modeling.Simulation` interface rather than importing the parent runner.
Hardware models (`mem`, `noc`) and tools (`monitoring`, `daisen`, `inspect`)
remain outside this directory. The runner has no dependency on either UI.

## How It Works

A `Simulation` is created through its `Builder`, which constructs and connects
the supporting infrastructure. Once built, you register components, connections,
and shared resources with the simulation; registration tracks each object in a
flat inventory and wires it into tracing and monitoring. The simulation exposes
accessors for runtime services such as `Engine`, `DataRecorder`,
`TaskTracker`, and `VisTracer`. The entity inventory is internal; keep the
component and port references returned during setup. After all ports and connections
are bound, call `s.Initialize()` to validate wiring and create component State
and Middlewares. This freezes topology; seed work and call `s.Run()` afterward.
`Simulation.Run() error` checks successful initialization. Direct `Engine.Run()`
and `SerialEngine.RunUntil(t)` require initialization but do not check it.
`RunUntil` is a serial-engine checkpoint/testing API, not part of `Engine` or
`Simulation`. Custom engine injection is deferred to v5.2.

## Key Concepts

- A `Simulation` always holds an `Engine` (serial or parallel). Recording,
  visual tracing, and monitoring are optional. The default simulation creates
  no database and no output files, including during Run and Terminate.
- Every registered runtime object — component, port, connection, and
  shared-state resource — is held in one internal flat inventory. Names are
  **globally unique across all kinds**.
- A `Resource` is non-timing program state (such as memory contents or page
  tables) that multiple components reference. The simulation owns resources;
  components hold references to them rather than embedding the payload.

## Builder Pattern

```go
monitor := monitoring.MakeBuilder().WithPortNumber(8080).Build()
s := sim.MakeBuilder().
    WithParallelEngine().       // optional: use the parallel engine
    WithMonitor(monitor).       // optional: enable live monitoring
    WithVisTracingOnStart().    // optional: start tracing immediately
    Build()
defer func() {
    if err := s.Terminate(); err != nil {
        panic(err)
    }
}()
```

| Method | Description |
|--------|-------------|
| `WithParallelEngine()` | Use `ParallelEngine` instead of `SerialEngine` |
| `WithMonitor(monitor)` | Attach an optional monitor; omitted by default |
| `WithOutputPath(path)` | Enable recording at this exact path; existing files are rejected |
| `WithVisTracingOnStart()` | Enable task tracking and visual tracing from time 0 |
| `WithTaskTracking()` | Enable in-memory task tracking; no recording by itself |

### Optional monitoring

`MakeBuilder().Build()` starts no monitoring server. To enable the web UI,
import `github.com/sarchlab/akita/v5/monitoring` and supply a monitor as shown
above. After successful `Initialize`, the runner calls `Start(s)` to bind and
start the monitor. The monitor discovers complete component and port inventories
and stops before those recording resources close in `Terminate`.
Each monitor belongs to one simulation and must not be reused.

Recording is enabled explicitly by `WithOutputPath`, `WithVisTracingOnStart`, or
`WithMonitor`. Without an explicit path, the latter two choose a unique SQLite
filename. Existing files are never overwritten. `WithTaskTracking` alone keeps
tracking in memory: `DataRecorder()` and `VisTracer()` remain nil. For manual
database tracing, combine `WithTaskTracking().WithOutputPath("trace.sqlite3")`,
then call `s.VisTracer().StartTracing()`. `WithVisTracingOnStart` starts it
automatically. A DB tracer exists only when both tracking and recording are on.
Monitors can access runtime services through `Engine()`, `DataRecorder()`,
`VisTracer()`, and `TraceDBPath()` without changing their lifecycle interface.

A custom monitor implements `sim.Monitor` with `Start(*sim.Simulation)`,
and `Stop()`. Discover objects through `s.Components()` and `s.Ports()`, which
return copied slices of live objects. Keep the concrete monitor in
application setup for implementation-specific features such as
`monitor.CreateProgressBar`.
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
tracker := s.TaskTracker() // nil when task tracking is off
```

## Checkpoint and Resume

A simulation can be checkpointed to a `.tar.gz` archive and resumed later — for
example, to snapshot between GPU kernels or to restart a long run. The contract
is an oracle: *running to the end* must equal *checkpoint, rebuild, restore, run
to the end*.

```go
// Save (engine stopped, outside an event handler):
err := s.SaveCheckpoint("snap.tar.gz")

// Resume: rebuild the *identical* simulation, bind its links, and successfully
// call s.Initialize() with the same setup code, then:
err := s.LoadCheckpoint("snap.tar.gz")
engine.Run()                                   // continue to completion
```

Setup rebuilds the *shape* (components, ports, connections, resources, wiring);
the checkpoint restores only the *runtime* (each entity's `State`, port buffers,
shared resources, the event queue, the engine time, and the ID-generator
counter). The archive holds one payload per entity; these files are the
inventory (there is no manifest). Loading validates the saved and rebuilt entity
sets, except for the optional task-tracker payload described below, and each
entity validates its payload, including component Spec hashes. No build identity
is stored or compared. Callers must ensure simulator code and saved state remain
compatible; successful decoding does not guarantee unchanged semantics.

Saving and loading reject active `Run` or `SerialEngine.RunUntil` calls, including
paused execution and calls through `s.Engine()`. The whole checkpoint operation
excludes execution startup. Checkpointing after `RunUntil` returns is supported.

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
- **Task tracking is checkpointed.** An enabled tracker restores open task
  identities and request-parent correlation. Ports restore private buffer task
  IDs and wait boundaries without allocating replacement IDs. Keep the tracking
  setting consistent across save and restore
  when comparing ID sequences; changing it changes future ID allocation. `Detail`
  and observer history, including earlier tags/milestones, are not restored. Mixed
  tracking settings are supported: off-to-on starts empty, and on-to-off ignores
  the optional tracker payload. See [task tracking](tasks/README.md).
- **`SerialEngine.RunUntil(t)`** stops the engine at a deterministic boundary
  (every event with time ≤ `t`), unlike `Run` (drains everything) or
  `RequestPause().Wait(ctx)` (acknowledges a pause at an execution boundary
  without choosing a reproducible virtual time) — useful for taking a mid-transaction
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

### Termination

`Terminate() error` rejects active execution, including paused runs, before
cleanup starts. Once execution returns, it stops monitoring, finishes trace and
metadata recording, records topology, and closes the owned recorder exactly once.
Repeated calls return the original cleanup result. Termination is final even on
failure; initialization, execution, and checkpoint operations then return errors.
Independently created data recorders must be closed by their callers; no exit
hook flushes them.
