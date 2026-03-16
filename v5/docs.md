# Akita V5 — Framework Documentation

Akita V5 is a discrete-event simulation (DES) framework for computer
architecture research, written in Go. It provides a generic, type-safe
component model with automatic checkpoint/restore, a middleware-based
behavioral pipeline, tracing infrastructure, and ready-made building blocks
for memory hierarchies and networks-on-chip.

> **Component authoring details** — For the full guide on writing Spec, State,
> Middlewares, and Builders, see
> [`component_guide.md`](../component_guide.md) at the repository root. This
> document provides the high-level architecture and usage overview; it
> intentionally does not duplicate the component guide.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [How to Write a Simulator — Step by Step](#2-how-to-write-a-simulator--step-by-step)
3. [Component Model Deep Dive](#3-component-model-deep-dive)
4. [Simulation Lifecycle](#4-simulation-lifecycle)
5. [Middleware Pipeline](#5-middleware-pipeline)
6. [Event-Driven vs Tick-Driven](#6-event-driven-vs-tick-driven)
7. [Key Types Reference](#7-key-types-reference)
8. [Tracing and Observation](#8-tracing-and-observation)
9. [Network-on-Chip](#9-network-on-chip)
10. [Memory Hierarchy](#10-memory-hierarchy)

---

## 1. Architecture Overview

### What is Akita V5?

Akita V5 is a **cycle-level / event-driven simulation framework** designed for
modeling computer architecture components — processors, caches, memory
controllers, interconnects, and more. Simulations are built by composing
*components* that communicate through *ports* over *connections*, all driven by
a central *engine*.

### Key Packages

| Package | Import Path | Purpose |
|---------|-------------|---------|
| **sim** | `akita/v5/sim` | Core engine, events, ports, connections, tick scheduling, ID generation, hooks |
| **modeling** | `akita/v5/modeling` | Generic `Component[S,T]` and `EventDrivenComponent[S,T]`, builders, save/load |
| **simulation** | `akita/v5/simulation` | High-level simulation runner, monitoring integration, checkpoint save/load |
| **queueing** | `akita/v5/queueing` | Serializable `Buffer[T]` and `Pipeline[T]` for use inside component state |
| **tracing** | `akita/v5/tracing` | Task-based tracing API, tracers (DB, average-time, busy-time, step-count) |
| **datarecording** | `akita/v5/datarecording` | SQLite-backed data recording for metrics and visualization |
| **noc** | `akita/v5/noc` | Network-on-chip: `directconnection`, `messaging`, `networking` (mesh, PCIe, NVLink) |
| **mem** | `akita/v5/mem` | Memory protocol (ReadReq/WriteReq), Storage, and subpackages for caches, DRAM, TLB, MMU |
| **monitoring** | `akita/v5/monitoring` | Live web-based monitoring server |

### Package Dependency Diagram

```
                    ┌──────────────┐
                    │  simulation  │  (runner, save/load, monitoring)
                    └──────┬───────┘
                           │ uses
          ┌────────────────┼────────────────┐
          │                │                │
    ┌─────▼─────┐   ┌─────▼─────┐   ┌──────▼───────┐
    │  modeling  │   │  tracing  │   │ datarecording│
    └─────┬─────┘   └─────┬─────┘   └──────────────┘
          │               │
          └───────┬───────┘
                  │ uses
             ┌────▼────┐
             │   sim   │  (core: engine, ports, events, freq, IDs)
             └────┬────┘
                  │ used by
       ┌──────────┼──────────┐
       │          │          │
  ┌────▼───┐ ┌───▼────┐ ┌───▼──────┐
  │  noc   │ │  mem   │ │ queueing │
  └────────┘ └────────┘ └──────────┘
```

---

## 2. How to Write a Simulator — Step by Step

This section walks through building a complete tick-based simulator from
scratch. For the event-driven variant, see [§6](#6-event-driven-vs-tick-driven).

### Step 1: Define Messages

Messages are the protocol between components. Every message embeds
`sim.MsgMeta` which carries routing metadata (source, destination, ID).

```go
package counter

import "github.com/sarchlab/akita/v5/sim"

// IncrementReq asks the counter to increment by Delta.
type IncrementReq struct {
    sim.MsgMeta
    Delta int
}

// IncrementRsp acknowledges the increment and returns the new value.
type IncrementRsp struct {
    sim.MsgMeta
    NewValue int
}
```

### Step 2: Define Spec (Immutable Configuration)

The Spec captures build-time parameters. It must contain **only primitives**
(bool, int, uint, float, string), slices of primitives, or maps with string
keys and primitive values. No pointers, interfaces, or nested structs.

```go
type Spec struct {
    MaxValue int      `json:"max_value"`
    Freq     sim.Freq `json:"freq"`
}
```

> See [`component_guide.md` §1.1](../component_guide.md) for the full Spec
> rules and validation.

### Step 3: Define State (Mutable Runtime Data)

The State captures everything that changes during simulation. It may contain
nested structs (unlike Spec), but no pointers, interfaces, or channels. All
fields must be JSON-serializable for checkpoint support.

```go
type State struct {
    Value               int                  `json:"value"`
    PendingTransactions []pendingTransaction `json:"pending_transactions"`
}

type pendingTransaction struct {
    ReqID  uint64         `json:"req_id"`
    ReqSrc sim.RemotePort `json:"req_src"`
    Delta  int            `json:"delta"`
}
```

> See [`component_guide.md` §1.2](../component_guide.md) for State rules.

### Step 4: Implement Middleware(s)

Middlewares implement per-tick behavior. Each middleware is a struct with a
`Tick() bool` method. Return `true` if progress was made.

```go
type processMW struct {
    comp *modeling.Component[Spec, State]
}

func (m *processMW) Tick() bool {
    port := m.comp.GetPortByName("Input")

    msg := port.PeekIncoming()
    if msg == nil {
        return false
    }

    req, ok := msg.(*IncrementReq)
    if !ok {
        return false
    }

    state := m.comp.GetNextState()
    spec := m.comp.GetSpec()

    newValue := state.Value + req.Delta
    if newValue > spec.MaxValue {
        newValue = spec.MaxValue
    }
    state.Value = newValue

    // Send response
    rsp := &IncrementRsp{
        MsgMeta: sim.MsgMeta{
            ID:    sim.GetIDGenerator().Generate(),
            Src:   port.AsRemote(),
            Dst:   req.Src,
            RspTo: req.ID,
        },
        NewValue: newValue,
    }

    err := port.Send(rsp)
    if err != nil {
        return false
    }

    port.RetrieveIncoming()
    return true
}
```

### Step 5: Write Builder

The Builder wires everything together. The pattern is: `MakeBuilder()` →
`With*()` setters → `Build(name)`.

```go
var DefaultSpec = Spec{
    MaxValue: 1000,
    Freq:     1 * sim.GHz,
}

type Builder struct {
    engine    sim.EventScheduler
    spec      Spec
    inputPort sim.Port
}

func MakeBuilder() Builder {
    return Builder{spec: DefaultSpec}
}

func (b Builder) WithEngine(e sim.EventScheduler) Builder {
    b.engine = e
    return b
}

func (b Builder) WithFreq(freq sim.Freq) Builder {
    b.spec.Freq = freq
    return b
}

func (b Builder) WithInputPort(p sim.Port) Builder {
    b.inputPort = p
    return b
}

func (b Builder) Build(name string) *modeling.Component[Spec, State] {
    comp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.spec.Freq).
        WithSpec(b.spec).
        Build(name)
    comp.SetState(State{})

    comp.AddMiddleware(&processMW{comp: comp})

    b.inputPort.SetComponent(comp)
    comp.AddPort("Input", b.inputPort)

    return comp
}
```

### Step 6: Wire Topology

Create ports externally, build components, and connect them with a
`directconnection`.

```go
engine := sim.NewSerialEngine()

// Create ports (comp=nil initially, set by builder)
senderPort := sim.NewPort(nil, 4, 4, "Sender.Out")
counterPort := sim.NewPort(nil, 4, 4, "Counter.Input")

// Build components
sender := senderBuilder.
    WithEngine(engine).
    WithOutPort(senderPort).
    Build("Sender")
counter := MakeBuilder().
    WithEngine(engine).
    WithInputPort(counterPort).
    Build("Counter")

// Connect
conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Conn")
conn.PlugIn(sender.GetPortByName("Out"))
conn.PlugIn(counter.GetPortByName("Input"))
```

### Step 7: Run Simulation

```go
// Set initial state (e.g., schedule work for sender)
// ...

// Kick off the first tick
sender.TickLater()

// Run until no more events
err := engine.Run()
if err != nil {
    log.Fatal(err)
}
```

### Complete Working Example

For a fully working example, see the `tickingping` example at
[`v5/examples/tickingping/`](examples/tickingping/), which demonstrates:
- Two agents ping-ponging messages
- Spec/State/Middleware/Builder pattern
- DirectConnection wiring
- The complete `Example()` test in `example_test.go`

---

## 3. Component Model Deep Dive

> **Full details:** See [`component_guide.md`](../component_guide.md) for
> exhaustive coverage of component anatomy, spec/state validation rules,
> middleware authoring, and builder patterns.

### Summary

Every V5 component is built from five orthogonal parts:

| Part | Purpose | Key Rule |
|------|---------|----------|
| **Spec** | Immutable config | Primitives only, JSON-serializable |
| **State** | Mutable runtime data | Nested structs OK, no pointers/interfaces, JSON-serializable |
| **Ports** | Communication endpoints | Created externally, injected via builder |
| **Middlewares** | Behavioral pipeline | `Tick() bool` — return true if progress made |
| **Hooks** | Observation points | Non-intrusive tracing/metrics/debugging |

### Tick-Based vs Event-Driven

| | `modeling.Component[S,T]` | `modeling.EventDrivenComponent[S,T]` |
|---|---|---|
| Scheduling | Periodic ticks at frequency | On-demand via `ScheduleWakeAt`/`ScheduleWakeNow` |
| Behavior | Middleware pipeline (`Tick() bool`) | Single `EventProcessor.Process()` |
| State update | In-place: `next = current` → middlewares → `current = next` | Direct mutation via `GetStatePtr()` |
| Good for | Pipeline-style components, caches | Latency-scheduled components, simple protocols |

---

## 4. Simulation Lifecycle

### Build Phase

1. **Create an engine** — either `sim.NewSerialEngine()` for deterministic
   single-threaded execution, or `sim.NewParallelEngine()` for multi-threaded.
2. **Build components** — use each component's Builder with `WithEngine()`,
   `WithFreq()`, etc.
3. **Create ports** — `sim.NewPort(comp, inBufCap, outBufCap, name)`.
   Ports are typically created with `comp=nil` and assigned during `Build()`.

### Connect Phase

4. **Create connections** — for simple topologies:
   ```go
   conn := directconnection.MakeBuilder().
       WithEngine(engine).
       WithFreq(1 * sim.GHz).
       Build("Conn")
   ```
5. **Plug in ports** — `conn.PlugIn(port)` for each port that should be
   reachable through this connection.

### Run Phase

6. **Set initial state** — configure component states (destinations, work items).
7. **Trigger first tick** — call `comp.TickLater()` on the component that
   should act first, or `comp.ScheduleWakeAt(t)` for event-driven components.
8. **Run** — `engine.Run()` processes all events until the queue is empty.

### Using the Simulation Runner

The `simulation` package provides a higher-level runner that integrates
engine creation, monitoring, and data recording:

```go
sim := simulation.MakeBuilder().
    WithParallelEngine().         // optional
    WithVisTracingOnStart().      // optional: enable visual tracing
    Build()
defer sim.Terminate()

// Build and register components
comp := myBuilder.WithEngine(sim.GetEngine()).Build("MyComp")
sim.RegisterComponent(comp)

// Connect, set initial state, then run
err := sim.GetEngine().Run()
```

### Checkpoint Save/Load

Akita V5 supports checkpointing via JSON serialization of component state.

**Saving** (all port buffers must be empty — quiescence):
```go
err := sim.Save("/path/to/checkpoint")
```

**Loading** (the simulation must be fully built first):
```go
err := sim.Load("/path/to/checkpoint")
// Components' tick schedulers are automatically reset.
// Trigger ticks on components that should resume:
comp.TickLater()
err = sim.GetEngine().Run()
```

For individual components:
```go
// Save
comp.SaveState(writer)
// Load
comp.LoadState(reader)
comp.ResetAndRestartTick()  // reset tick scheduler + schedule immediate tick
```

---

## 5. Middleware Pipeline

Middlewares implement the per-tick behavioral logic of a `modeling.Component`.

### How It Works

1. The engine fires a `TickEvent` for the component.
2. `Component.Tick()` is called:
   - `next = current` (shallow copy)
   - Each middleware's `Tick()` is called **in registration order**
   - `current = next` (commit)
3. If **any** middleware returned `true` (progress), a new tick is scheduled
   via `TickLater()`.
4. If **no** middleware made progress, ticking stops. The component is woken
   again when a port receives a message (`NotifyRecv`) or a port becomes free
   (`NotifyPortFree`).

### State Access Pattern

Inside a middleware:
```go
func (m *myMW) Tick() bool {
    state := m.comp.GetNextState()  // pointer to next state — mutate directly
    spec  := m.comp.GetSpec()       // read-only spec

    // Read from state, mutate *state, read spec
    state.Counter++

    return true // made progress
}
```

Because Akita V5 uses **in-place state update** (current and next refer to the
same underlying value), `GetState()` and `GetNextState()` see the same data.
The `next = current` assignment at tick start is a shallow copy for consistency.

### Multiple Middlewares

Middlewares execute in the order they are added via `comp.AddMiddleware()`.
All middlewares run every tick, and **all** of their return values are OR'd
together to determine progress.

```go
comp.AddMiddleware(&sendMW{comp: comp})         // runs first
comp.AddMiddleware(&receiveProcessMW{comp: comp}) // runs second
```

---

## 6. Event-Driven vs Tick-Driven

### When to Use Each

| Use Tick-Driven (`Component[S,T]`) | Use Event-Driven (`EventDrivenComponent[S,T]`) |
|---|---|
| The component has pipeline-style processing | The component reacts to discrete events with variable timing |
| Multiple middleware stages need to run each cycle | A single `Process()` function handles all logic |
| Cache controllers, pipeline stages, switches | Simple request-response protocols, delayed operations |

### Event-Driven Component

Instead of periodic ticking, an `EventDrivenComponent` wakes up only when:
- A message arrives on a port → `NotifyRecv` → `ScheduleWakeNow()`
- A port becomes free → `NotifyPortFree` → `ScheduleWakeNow()`
- The component schedules a future wakeup → `ScheduleWakeAt(time)`

The component implements `EventProcessor[S,T]`:

```go
type MyProcessor struct{}

func (p *MyProcessor) Process(
    comp *modeling.EventDrivenComponent[MySpec, MyState],
    now sim.VTimeInSec,
) bool {
    state := comp.GetStatePtr()
    spec := comp.GetSpec()

    // Process incoming messages, schedule future work, etc.
    // Use comp.ScheduleWakeAt(futureTime) for delayed actions.

    return true // made progress
}
```

**Dedup guard:** `ScheduleWakeAt` uses an internal guard (`pendingWakeup`) to
avoid scheduling redundant events. If a wakeup is already pending at an equal
or earlier time, the call is a no-op.

### Building an Event-Driven Component

```go
comp := modeling.NewEventDrivenBuilder[MySpec, MyState]().
    WithEngine(engine).
    WithSpec(mySpec).
    WithProcessor(&MyProcessor{}).
    Build("MyComp")
```

See the `ping` example at [`v5/examples/ping/`](examples/ping/) for a complete
event-driven component.

---

## 7. Key Types Reference

### `sim.VTimeInSec`

```go
type VTimeInSec uint64  // time in picoseconds (despite the legacy name)
```

All simulation time is measured in **picoseconds** as unsigned 64-bit integers.
1 second = 1,000,000,000,000 ps.

### `sim.Freq`

```go
type Freq uint64  // frequency in Hz

const (
    Hz  Freq = 1
    KHz Freq = 1e3
    MHz Freq = 1e6
    GHz Freq = 1e9
)
```

Key methods:
- `f.Period()` → `VTimeInSec` — picoseconds between ticks. E.g., `(1*GHz).Period()` = 1000 ps.
- `f.NextTick(now)` → next tick boundary after `now`.
- `f.ThisTick(now)` → current tick boundary (ceil to nearest).
- `f.NCyclesLater(n, now)` → time after N cycles from current tick.

### `sim.MsgMeta`

```go
type MsgMeta struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64     // non-zero if this is a response
    SendTaskID   uint64     // tracing: task ID at sender
    RecvTaskID   uint64     // tracing: task ID at receiver
}
```

Every message type embeds `MsgMeta` and implements `sim.Msg` automatically
(via `Meta() *MsgMeta`).

### `sim.IDGenerator`

```go
sim.GetIDGenerator().Generate()  // returns uint64
```

Generates globally unique uint64 IDs. Two implementations:
- **Sequential** (default) — deterministic, single atomic counter.
- **Parallel** — `sim.UseParallelIDGenerator()` for non-deterministic parallel runs.

Call `sim.UseSequentialIDGenerator()` or `sim.UseParallelIDGenerator()` before
any ID is generated. Once the generator is instantiated it cannot be changed.

### `sim.Port`

```go
// Create a port
port := sim.NewPort(comp, inBufCap, outBufCap, name)

// Component-side API
port.CanSend() bool
port.Send(msg) *SendError
port.PeekIncoming() Msg
port.RetrieveIncoming() Msg

// Connection-side API
port.Deliver(msg) *SendError
port.RetrieveOutgoing() Msg
port.PeekOutgoing() Msg
```

`RemotePort` is a `string` type alias used for routing:
```go
type RemotePort string
port.AsRemote() RemotePort  // returns the port's name as a RemotePort
```

### `sim.Engine`

```go
type Engine interface {
    Hookable
    EventScheduler          // Schedule(Event), CurrentTime()
    Run() error
    Pause()
    Continue()
}
```

Implementations:
- `sim.NewSerialEngine()` — deterministic, single-threaded.
- `sim.NewParallelEngine()` — multi-threaded for performance.

### `sim.Event`

```go
type Event interface {
    Time() VTimeInSec
    HandlerID() string       // dispatched to registered handler
    IsSecondary() bool       // secondary events run after same-time primaries
}
```

Use `sim.NewEventBase(time, handlerID)` to create event bases. Handlers are
registered on the engine via `engine.RegisterHandler(name, handler)`.

---

## 8. Tracing and Observation

### Task-Based Tracing API

The `tracing` package provides a **task lifecycle** model for tracking work
through the simulation.

```go
import "github.com/sarchlab/akita/v5/tracing"

// Start a top-level task
taskID := sim.GetIDGenerator().Generate()
tracing.StartTask(taskID, 0, domain, "kind", "what", detail)

// Add a step to a task
tracing.AddTaskStep(taskID, domain, "step description")

// End a task
tracing.EndTask(taskID, domain)
```

**Message tracing helpers** (for request-response patterns):

```go
// Sender side: when initiating a request
tracing.TraceReqInitiate(msg, domain, parentTaskID)

// Receiver side: when receiving a request
tracing.TraceReqReceive(msg, domain)

// Receiver side: when completing request processing
tracing.TraceReqComplete(msg, domain)

// Sender side: when receiving the response
tracing.TraceReqFinalize(msg, domain)
```

These helpers automatically manage `SendTaskID` / `RecvTaskID` on `MsgMeta`.

### Milestones

Milestones record points in time where a task's blocking status is resolved:

```go
tracing.AddMilestone(taskID, tracing.MilestoneKindNetworkTransfer,
    "data arrived", "Switch.Port1", domain)
```

Milestone kinds: `MilestoneKindHardwareResource`, `MilestoneKindNetworkTransfer`,
`MilestoneKindNetworkBusy`, `MilestoneKindQueue`, `MilestoneKindData`,
`MilestoneKindDependency`, `MilestoneKindTranslation`, `MilestoneKindSubTask`,
`MilestoneKindOther`.

### Hooks

Hooks provide non-intrusive observation points. Any `Hookable` object
(components, ports, engines) can accept hooks:

```go
type MyHook struct{}

func (h *MyHook) Func(ctx sim.HookCtx) {
    // ctx.Domain — the hookable that triggered
    // ctx.Pos    — hook position (e.g., HookPosBeforeEvent)
    // ctx.Item   — associated data (event, message, task, etc.)
}

engine.AcceptHook(&MyHook{})
port.AcceptHook(&MyHook{})
```

**Standard hook positions:**
- Engine: `sim.HookPosBeforeEvent`, `sim.HookPosAfterEvent`
- Port: `sim.HookPosPortMsgSend`, `sim.HookPosPortMsgRecvd`,
  `sim.HookPosPortMsgRetrieveIncoming`, `sim.HookPosPortMsgRetrieveOutgoing`
- Connection: `sim.HookPosConnStartSend`, `sim.HookPosConnStartTrans`,
  `sim.HookPosConnDoneTrans`, `sim.HookPosConnDeliver`
- Tracing: `tracing.HookPosTaskStart`, `tracing.HookPosTaskStep`,
  `tracing.HookPosMilestone`, `tracing.HookPosTaskEnd`

### Connecting Tracers

```go
// Attach a tracer to a component (or any NamedHookable)
tracing.CollectTrace(component, tracer)
```

Built-in tracers:
- `tracing.NewDBTracer(timeTeller, dataRecorder)` — persists traces to SQLite via
  the `datarecording` package.
- `tracing.NewAverageTimeTracer(...)` — calculates average task durations.
- `tracing.NewBusyTimeTracer(...)` — tracks component busy time.
- `tracing.NewStepCountTracer(...)` — counts task steps.

### Data Recording

The `datarecording` package provides a `DataRecorder` interface backed by
SQLite:

```go
recorder := datarecording.NewDataRecorder("output_path")
defer recorder.Close()

recorder.CreateTable("my_metrics", sampleEntry)
recorder.InsertData("my_metrics", entry)
recorder.Flush()
```

---

## 9. Network-on-Chip

### Direct Connection (Simple Topologies)

`directconnection` is a zero-latency connection for simple point-to-point or
bus-like topologies. Messages are forwarded in the same tick.

```go
import "github.com/sarchlab/akita/v5/noc/directconnection"

conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Conn")

conn.PlugIn(portA)
conn.PlugIn(portB)
conn.PlugIn(portC)
// Any port can now send to any other port in this connection.
```

Direct connections implement `sim.Connection` and handle round-robin
forwarding across all plugged-in ports. They are implemented as secondary
ticking components (secondary events are processed after same-time primary
events).

### Networking Package (Complex Topologies)

For realistic network simulation with switches, routing, and link modeling,
use the `noc/networking` subpackages:

| Subpackage | Purpose |
|------------|---------|
| `networking/mesh` | Mesh topology builder with configurable dimensions |
| `networking/pcie` | PCIe-style hierarchical interconnect |
| `networking/nvlink` | NVLink-style high-bandwidth interconnect |
| `networking/switching` | Switch components (endpoint + switches with pipeline stages) |
| `networking/routing` | Routing table construction |
| `networking/networkconnector` | Connector utilities |

The `noc/messaging` package defines the `Flit` (flow control digit) structure
used by the network switches.

**Switching components** are full-featured Akita V5 components with Spec/State
and middlewares:
- **Endpoint** (`switching/endpoint`) — adapts component ports to the network.
- **Switch** (`switching/switches`) — routes flits through a receive pipeline,
  then forwards them via routing tables.

---

## 10. Memory Hierarchy

### Memory Protocol

The `mem` package defines the standard memory access protocol:

```go
import "github.com/sarchlab/akita/v5/mem"

// Requests
&mem.ReadReq{Address: 0x1000, AccessByteSize: 64, PID: pid}
&mem.WriteReq{Address: 0x1000, Data: data, DirtyMask: mask, PID: pid}

// Responses
&mem.DataReadyRsp{Data: data}   // response to ReadReq
&mem.WriteDoneRsp{}             // response to WriteReq

// Control
&mem.ControlReq{Command: mem.CmdFlush, InvalidateAfter: true}
&mem.ControlRsp{Command: mem.CmdFlush, Success: true}
```

Control commands: `CmdFlush`, `CmdInvalidate`, `CmdDrain`, `CmdReset`,
`CmdPause`, `CmdEnable`.

### Storage

`mem.Storage` is a sparse, page-based byte store:

```go
storage := mem.NewStorage(4 * mem.GB)
storage.Write(0x1000, data)
readData, _ := storage.Read(0x1000, 64)
```

Capacity constants: `mem.KB`, `mem.MB`, `mem.GB`, `mem.TB`.

### Available Memory Components

| Component | Path | Description |
|-----------|------|-------------|
| **Ideal Memory Controller** | `mem/idealmemcontroller` | Fixed-latency memory controller with `mem.Storage` |
| **Writeback Cache** | `mem/cache/writeback` | Write-back L1/L2 cache with MSHR support |
| **Writethrough Cache** | `mem/cache/writethroughcache` | Write-through cache |
| **DRAM** | `mem/dram` | Detailed DRAM timing model |
| **TLB** | `mem/vm/tlb` | Translation lookaside buffer |
| **MMU** | `mem/vm/mmu` | Memory management unit with page table walks |
| **Address Mapper** | `mem` | `AddressToPortMapper` for address-based routing |
| **Data Mover** | `mem/datamover` | DMA-like data transfer component |
| **Simple Banked Memory** | `mem/simplebankedmemory` | Multi-bank memory with address interleaving |

All memory components follow the standard Akita V5 component model (Spec,
State, Builder) and communicate via the `mem` protocol messages.

### Address-to-Port Mapping

The `mem` package provides `AddressToPortMapper` implementations for routing
requests to the correct low-level module:

```go
// Interleaved: addresses are striped across modules
mapper := mem.NewInterleavedAddressPortMapper(interleavingSize)
mapper.LowModules = append(mapper.LowModules,
    bank0Port.AsRemote(), bank1Port.AsRemote())
dst := mapper.Find(address)  // returns sim.RemotePort

// Banked: each module owns a contiguous address bank
mapper := mem.NewBankedAddressPortMapper(bankSize)
mapper.LowModules = append(mapper.LowModules,
    bank0Port.AsRemote(), bank1Port.AsRemote())

// Single: all addresses go to one module
mapper := &mem.SinglePortMapper{Port: memPort.AsRemote()}
```
