---
sidebar_position: 7
---

# V4 → V5 Migration Guide

This guide covers all breaking changes between Akita V4 and V5. Each section
explains the motivation, shows before/after code, and notes pitfalls.

---

## Table of Contents

1. [Integer Time](#1-integer-time)
2. [uint64 Entity IDs](#2-uint64-entity-ids)
3. [Unified Control Protocol](#3-unified-control-protocol)
4. [Event Serialization: Handler() → HandlerID()](#4-event-serialization-handler--handlerid)
5. [In-Place State Update](#5-in-place-state-update)
6. [Component Models: Five Structs + Definition](#6-component-model)
7. [DRAM Improvements](#7-dram-improvements)
8. [Port Creation API](#8-port-creation-api)
9. [Queueing V5](#9-queueing-v5)
10. [CLI Changes](#10-cli-changes)
11. [CI Migration](#11-ci-migration)

---

## 1. Integer Time

**Motivation:** Floating-point time (`float64` seconds) caused subtle
rounding errors and non-determinism. V5 switches to integer picoseconds
for exact, reproducible arithmetic.

### Type Changes

| V4 | V5 |
|----|-----|
| `type VTimeInSec float64` (seconds) | `type VTimeInPicoSec uint64` (picoseconds) |
| `type Freq float64` (cycles/second) | `type Freq uint64` (Hz) |

V5 renames the type to make the unit explicit. The old V4 name `VTimeInSec`
no longer exists in V5.

### Freq Constants

```go
// v5/sim/freq.go
const (
    Hz  Freq = 1
    KHz Freq = 1e3
    MHz Freq = 1e6
    GHz Freq = 1e9
)
```

### Freq Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `Period()` | `func (f Freq) Period() VTimeInPicoSec` | Picoseconds per cycle. 1 GHz → 1000 ps. |
| `Cycle()` | `func (f Freq) Cycle(time VTimeInPicoSec) uint64` | Number of complete cycles since time 0. |
| `ThisTick()` | `func (f Freq) ThisTick(now VTimeInPicoSec) VTimeInPicoSec` | Ceil to nearest tick boundary. |
| `NextTick()` | `func (f Freq) NextTick(now VTimeInPicoSec) VTimeInPicoSec` | Next tick strictly after `now`. |
| `NCyclesLater()` | `func (f Freq) NCyclesLater(n int, now VTimeInPicoSec) VTimeInPicoSec` | Time `n` cycles from current tick. |
| `HalfTick()` | `func (f Freq) HalfTick(t VTimeInPicoSec) VTimeInPicoSec` | Midpoint between two ticks. |

### Before / After

**Before (V4) — float64 seconds:**
```go
freq := sim.Freq(1e9) // 1 GHz as float64

now := sim.VTimeInPicoSec(0.000000001) // 1 nanosecond
period := 1.0 / float64(freq)      // 1e-9 seconds
next := now + sim.VTimeInPicoSec(period)

// Danger: float rounding errors accumulate over millions of cycles.
```

**After (V5) — uint64 picoseconds:**
```go
freq := 1 * sim.GHz // Freq(1_000_000_000)

now := sim.VTimeInPicoSec(1000) // 1000 ps = 1 ns
period := freq.Period()      // 1000 ps
next := freq.NextTick(now)   // 2000 ps

// Integer arithmetic: exact, deterministic, no rounding errors.
```

### Migration Checklist

- Replace all `float64` time arithmetic (`+`, `-`, `*`, `/`) with `uint64` arithmetic or `Freq` helper methods.
- Replace `Freq(1e9)` with `1 * sim.GHz`.
- Replace `1.0 / float64(freq)` with `freq.Period()`.
- Replace `math.Ceil(now * freq) / freq` patterns with `freq.ThisTick(now)`.
- Replace `now + period` with `freq.NextTick(now)`.
- Time comparisons change from `<` on float64 to `<` on uint64 — this is safe as-is.
- Any `time == 0.0` checks become `time == 0`.

---

## 2. uint64 Entity IDs

**Motivation:** String IDs (UUIDs) were expensive to generate, compare,
hash, and serialize. V5 uses monotonically increasing `uint64` values.

### Type Changes

| V4 | V5 |
|----|-----|
| `MsgMeta.ID string` | `MsgMeta.ID uint64` |
| `MsgMeta.RspTo string` | `MsgMeta.RspTo uint64` |
| `IDGenerator.Generate() string` | `IDGenerator.Generate() uint64` |
| `tracing.Task.ID string` | `tracing.Task.ID uint64` |
| `tracing.Task.ParentID string` | `tracing.Task.ParentID uint64` |
| Empty/nil sentinel: `""` | Empty/nil sentinel: `0` |

### MsgMeta (V5)

```go
// v5/sim/msg.go
type MsgMeta struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64
    SendTaskID   uint64 `json:"send_task_id"`
    RecvTaskID   uint64 `json:"recv_task_id"`
}

// IsRsp returns true if this message is a response.
func (m *MsgMeta) IsRsp() bool { return m.RspTo != 0 }
```

Note the new `SendTaskID` and `RecvTaskID` fields for tracing integration.

### IDGenerator (V5)

Each simulation owns its counter. Components retain their simulation reference. There
is no process-global generator or sequential/parallel configuration switch.

```go
id := sim.NewID() // uint64, unique within this simulation
req.ID = component.NewID() // uses the same counter
```

Event factories now take the allocated ID explicitly:
`timing.MakeEventBase(sim.NewID(), time, handlerID)` and
`ticking.MakeTickEvent(sim.NewID(), handlerID, time)`.

Separate simulations may reuse numeric IDs. Tracing associations are keyed by
the component itself and the message ID, so simulations that reuse IDs stay
apart.

Pass the simulation to builders with `WithSimulation(sim)`. All component and
package builders accept the shared `timing.Simulation` interface. Components expose `Simulation()`, while engines and components have
no `NewID()` method. For lightweight setups, create
`modeling.NewStandaloneSimulation(engine)` once and share that instance with
all builders. Custom components and tracing domains implement
`Simulation() timing.Simulation`. Monitors use `RegisterSimulation(sim)` so
progress IDs come from the same counter.

### Before / After

**Before (V4) — string IDs:**
```go
pendingReqs := map[string]*ReadReq{}

req := &ReadReq{}
req.ID = sim.GetIDGenerator().Generate() // "abc-123-..."
pendingReqs[req.ID] = req

// Later, matching response:
if original, ok := pendingReqs[rsp.RspTo]; ok {
    delete(pendingReqs, rsp.RspTo)
}

// Check if ID is empty:
if req.ID == "" { ... }
```

**After (V5) — uint64 IDs:**
```go
pendingReqs := map[uint64]*ReadReq{}

req := &ReadReq{}
req.ID = component.NewID() // 1, 2, 3, ...
pendingReqs[req.ID] = req

// Later, matching response:
if original, ok := pendingReqs[rsp.RspTo]; ok {
    delete(pendingReqs, rsp.RspTo)
}

// Check if ID is empty:
if req.ID == 0 { ... }
```

### Migration Checklist

- Change `map[string]...` keyed on IDs to `map[uint64]...`.
- Replace `== ""` / `!= ""` checks with `== 0` / `!= 0`.
- Replace `fmt.Sprintf`-based ID formatting with `strconv.FormatUint` or `%d`.
- Update tracing task ID comparisons from string to uint64.
- Replace global ID allocation with `sim.NewID()` or `component.NewID()`.
- Simulation checkpoints include the owned counter. Standalone simulation users must checkpoint `sim.GetIDGenerator()` alongside the engine. Restore into fresh instances.

---

## 3. Unified Control Protocol

**Motivation:** V4 had separate request/response types for each control
operation (flush, drain, restart). V5 consolidates them into a single
`ControlReq` / `ControlRsp` pair with a `Command` enum.

### V5 Types

```go
// v5/mem/protocol.go
type ControlCommand int

const (
    CmdFlush      ControlCommand = iota // Write back dirty data
    CmdInvalidate                       // Invalidate entries without writeback
    CmdDrain                            // Wait for in-flight ops to complete
    CmdReset                            // Soft reset
    CmdPause                            // Disable further processing
    CmdEnable                           // Re-enable processing
)

type ControlReq struct {
    sim.MsgMeta
    Command         ControlCommand
    DiscardInflight bool     // For Flush: discard vs wait for in-flight
    InvalidateAfter bool     // For Flush: invalidate lines after writeback
    PauseAfter      bool     // For Flush/Drain: pause after completion
    Addresses       []uint64 // For Invalidate: specific addresses (empty = all)
    PID             vm.PID   // For Invalidate: process filter
}

type ControlRsp struct {
    sim.MsgMeta
    Command ControlCommand
    Success bool
}
```

### Before / After

**Before (V4) — separate types:**
```go
// Flushing a cache
flushReq := &cache.FlushReq{}
flushReq.Src = controlPort
flushReq.Dst = cacheControlPort
engine.Send(flushReq)

// Draining a cache
drainReq := &cache.DrainReq{}
drainReq.Src = controlPort
drainReq.Dst = cacheControlPort
engine.Send(drainReq)

// Restarting a cache
restartReq := &cache.RestartReq{}
restartReq.Src = controlPort
restartReq.Dst = cacheControlPort
engine.Send(restartReq)

// Handler needed separate cases for FlushRsp, DrainRsp, RestartRsp
```

**After (V5) — unified ControlReq:**
```go
// Flushing a cache
flushReq := &mem.ControlReq{
    Command:         mem.CmdFlush,
    InvalidateAfter: true,
}
flushReq.Src = controlPort
flushReq.Dst = cacheControlPort
engine.Send(flushReq)

// Draining a cache
drainReq := &mem.ControlReq{
    Command:    mem.CmdDrain,
    PauseAfter: true,
}
drainReq.Src = controlPort
drainReq.Dst = cacheControlPort
engine.Send(drainReq)

// Re-enabling after drain
enableReq := &mem.ControlReq{
    Command: mem.CmdEnable,
}
enableReq.Src = controlPort
enableReq.Dst = cacheControlPort
engine.Send(enableReq)

// Handler uses single ControlRsp type:
func handleControlRsp(rsp *mem.ControlRsp) {
    switch rsp.Command {
    case mem.CmdFlush:
        // flush completed
    case mem.CmdDrain:
        // drain completed
    case mem.CmdEnable:
        // re-enabled
    }
}
```

### Migration Checklist

- Replace `FlushReq`/`FlushRsp` with `ControlReq{Command: mem.CmdFlush}` / `ControlRsp`.
- Replace `DrainReq`/`DrainRsp` with `ControlReq{Command: mem.CmdDrain}` / `ControlRsp`.
- Replace `RestartReq`/`RestartRsp` with `ControlReq{Command: mem.CmdEnable}` or `CmdReset` / `ControlRsp`.
- Update type switches in message handlers to check `ControlRsp.Command`.
- Use `DiscardInflight`, `InvalidateAfter`, `PauseAfter` flags for fine-grained flush/drain behavior.

---

## 4. Event Serialization: Handler() → HandlerID()

**Motivation:** V4 events held a direct Go interface reference to their
handler (`Handler() Handler`). This prevented JSON serialization of the
event queue, which is needed for checkpoint/restore.

V5 events store only a string handler ID. The engine looks up the
concrete handler in a registry at dispatch time.

### V5 Event Interface

```go
// v5/sim/event.go
type Event interface {
    Time() VTimeInPicoSec
    HandlerID() string    // was Handler() Handler in V4
    IsSecondary() bool
}

type EventBase struct {
    ID         uint64     `json:"id"`
    Time_      VTimeInPicoSec `json:"time"`
    HandlerID_ string     `json:"handler_id"`
    Secondary  bool       `json:"secondary"`
}
```

### Handler Registration

The engine implements `HandlerRegistry`:

```go
// v5/sim/engine.go
type HandlerRegistry interface {
    RegisterHandler(name string, handler Handler)
}
```

Components register themselves during construction. Every component model's
`Build` ends by registering the instance (`Register` in
`modeling/internal/base`): it registers the instance's ports,
registers the instance with the engine under its name (so events whose
`HandlerID()` is that name reach it), and registers it with the simulation:

```go
// v5/modeling/internal/base/base.go
func Register[S, T, R, P, M any](base *ComponentBase[S, T, R, P, M]) {
    registerPorts(base.simulation, &base.Ports)

    if handlers, ok := base.simulation.GetEngine().(timing.HandlerRegistry); ok {
        handlers.RegisterHandler(base.name, base.owner)
    }

    base.simulation.RegisterComponent(base.owner)
}
```

### Before / After

**Before (V4) — interface reference:**
```go
type Event interface {
    Time() VTimeInPicoSec
    Handler() Handler    // direct pointer to handler
    IsSecondary() bool
}

// Creating an event:
evt := sim.NewEventBase(now, myComponent) // passed handler object
```

**After (V5) — string ID + registry:**
```go
type Event interface {
    Time() VTimeInPicoSec
    HandlerID() string   // string name, serializable
    IsSecondary() bool
}

// Creating an event:
evt := timing.MakeEventBase(sim.NewID(), now, "MyComponent") // pass handler name
```

### Migration Checklist

- Replace `evt.Handler()` calls with `evt.HandlerID()`.
- Replace `NewEventBase(time, handlerObj)` with `timing.MakeEventBase(id, time, "handlerName")`.
- Ensure all event handlers are registered with the engine via `RegisterHandler`.
- Custom event types: change the `Handler` field from interface to `string`.

---

## 5. Single-State Update

**Motivation:** V4 used a double-buffered model where `current` and `next`
were deep copies. This was expensive and error-prone. V5 simplifies to
one mutable state value per component.

### V5 Dispatch

A V5 component hands each event it receives to its middlewares, which mutate
its one State in place. From `v5/modeling/ticking/component.go`:

```go
func (c *Component[S, T, R, P, M]) Handle(e timing.Event) {
    if base.Dispatch(c.pipeline, e) {
        c.ticks.TickLater()
    }
}
```

Components store one state value, the exported `State` field. A middleware
mutates it in place through a pointer, and the change is immediately visible
to every middleware that runs after it.

### State Access

```go
// Inside a middleware: mutate the component's State in place.
state := &m.comp.State
state.Counter++

// Anywhere: read a copy, for example in a test or a report.
snapshot := comp.State
```

The initial State comes from the `Definition`'s `NewState` (or is the zero
value), and loading a checkpoint replaces it wholesale. Only the component's
own code writes it.

### Before / After

**Before (V4) — deep copy double buffer:**
```go
// V4: current and next were separate deep copies.
// Reading current gave the pre-tick snapshot.
// Writing next didn't affect current until commit.
state := comp.State     // snapshot from start of tick
next := &comp.State  // separate copy
next.Value = state.Value + 1 // must read from state, write to next
comp.CommitNextState()        // deep copy next → current
```

**After (V5) — single-state update:**
```go
// Inside a middleware: read and write through a pointer to the State field.
state := &m.comp.State
state.Value++                  // direct mutation, visible immediately

// No explicit commit needed.
```

### Migration Checklist

- Remove any deep-copy logic between current/next state.
- Mutate `State` in place through a pointer (`&m.comp.State`) from the
  component's middlewares; read a copy with `comp.State`.
- For initialization, return the initial State from the `Definition`'s
  `NewState`.
- For checkpoint restore, write nothing: the component model's
  `LoadCheckpoint` replaces the State.

---

## 6. Component Model

**Motivation:** In V4 every component was a hand-written struct: it embedded
the engine's ticking base, created its own ports in a per-package builder, and
implemented a single per-tick method. V5 defines every component the same way
— five structs and a `Definition` in a package of its own — and a **component
model** supplies everything else: scheduling, port binding, registration, and
checkpointing. See "Defining Components in V5" below for the full philosophy.

### Anatomy

| Struct | What it is | Supplied by | Key rule |
|--------|------------|-------------|----------|
| **Spec** | Configuration | System builder (defaults in `Definition.DefaultSpec`) | Scalar fields only (bool, numbers, strings, and named types based on them). No slices, arrays, maps, nested structs, pointers, or interfaces. A ticking component's Spec has a `Freq timing.Freq` field. |
| **State** | Mutable runtime data, saved in checkpoints | Component (`NewState`, or the zero value) | Pure data: scalars, slices, arrays, maps, nested structs. No pointers, ports, functions, channels. Use IDs for cross-references. Written only by the component's own code. |
| **Resources** | References to shared objects (storage, page table, address mapper) | System builder | Not checkpointed; the rebuild supplies them again. `modeling.None` when there are none. |
| **Ports** | One `messaging.Port` field per port, `[]messaging.Port` per port group | System builder (`messaging.NewPort`) | Bound and registered by `Build`; none is added later. A field may carry an `akita:"role=<protocol>.<role>"` tag. |
| **Middlewares** | The behavior: one exported pointer field per middleware | Component (`NewMiddlewares`) | Each implements `Handle(e timing.Event) bool`; they run in field order and hold only references. |

Hooks are not a sixth struct: every component embeds `hooking.HookableBase`,
so it accepts hooks and fires its own hook points (`InvokeHook`) without
affecting simulation logic.

### Three Component Models

Each model is a sub-package of `modeling` with the same shape — a `Definition`,
a `Builder`, and a generic `Component` over the five structs. They differ only
in which events reach the middlewares:

| Model | Events that reach the middlewares | Use it for |
|-------|-----------------------------------|------------|
| `modeling/ticking` | a `ticking.TickEvent` every cycle while any middleware makes progress; port activity restarts ticking (`TickLater` starts it) | work that advances cycle by cycle: pipelines, caches, switches. The default, and the usual target for a V4 ticking component. |
| `modeling/wakeup` | a data-less `wakeup.Event` on port activity, or at a time a middleware asked for with `WakeAt` | a component that is idle most of the time and knows when it next has work |
| `modeling/event` | `event.Recv` and `event.PortFree` for port activity, and the events the component schedules for itself with `Schedule` | behavior that is a set of distinct happenings, each with its own data and time |

The *Wakeup and Event Components* tutorial walks through the two clockless
models.

### Declaring a Component

The reorder buffer in `mem/rob` is a small, complete example. Its package
declares the five structs and the `Comp` alias (Resources is `modeling.None`):

```go
// mem/rob/comp.go
type Spec struct {
    Freq           timing.Freq `json:"freq"`
    BufferSize     int         `json:"buffer_size"`
    NumReqPerCycle int         `json:"num_req_per_cycle"`

    // BottomUnit is the remote port of the unit that the reorder buffer
    // forwards requests to. The reorder buffer rewrites the Dst of every
    // shadow request to this value.
    BottomUnit messaging.RemotePort `json:"bottom_unit"`
}

type State struct {
    Transactions  []transactionState       `json:"transactions"`
    ControlState  memcontrolprotocol.State `json:"control_state"`
    CurrentCmdID  uint64                   `json:"current_cmd_id"`
    CurrentCmdSrc messaging.RemotePort     `json:"current_cmd_src"`
}

type Ports struct {
    Top     messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
    Bottom  messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
    Control messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder"`
}

type Middlewares struct {
    Pipeline *middleware
}

type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]
```

and its `Definition`:

```go
// mem/rob/definition.go
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
    DefaultSpec: Spec{
        Freq:           1 * timing.GHz,
        BufferSize:     128,
        NumReqPerCycle: 4,
    },
    NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
    return Middlewares{Pipeline: &middleware{comp: c}}
}
```

A middleware holds the component and implements `Handle`:

```go
// mem/rob/middleware.go
func (m *middleware) Handle(_ timing.Event) bool {
    madeProgress := false

    madeProgress = m.processControlMsg() || madeProgress

    switch m.comp.State.ControlState {
    case memcontrolprotocol.StateEnabled, memcontrolprotocol.StateDraining:
        madeProgress = m.runPipeline() || madeProgress
    }

    return madeProgress
}
```

`DefaultSpec` must be a keyed literal with constant leaves, and `NewState` and
`NewMiddlewares` must name functions, because the `inspect` package reads the
`Definition` without running the code. A one-line test keeps the static and
runtime views in agreement:

```go
// mem/rob/definition_test.go
func TestDefinitionMatchesSource(t *testing.T) {
    modelingtest.CheckTicking(t, Definition)
}
```

`Comp` is an alias of a type from another package, so Go allows no methods on
it. What used to be an exported method becomes a package function that takes
the instance — `ping.SchedulePing(comp, sendAt, dst)` in `examples/ping` — and
internal helpers become methods of the middlewares.

### Building a Component

The **system builder** — the code that assembles a simulation — creates every
port and builds the instance:

```go
spec := rob.Definition.DefaultSpec
spec.BottomUnit = bottomUnit.AsRemote()

r := rob.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithPorts(rob.Ports{
        Top:     messaging.NewPort("ROB.Top", 4, 4),
        Bottom:  messaging.NewPort("ROB.Bottom", 4, 4),
        Control: messaging.NewPort("ROB.Control", 4, 4),
    }).
    Build("ROB")

conn.PlugIn(r.Ports.Top)
```

`Build` validates the Spec and State, binds each port to the instance
(checking that it is named `<instance>.<Field>`, or `<instance>.<Field>[i]` for
member `i` of a port group), creates the State and the middlewares, and
registers the ports and the instance with the simulation. No port or middleware
is added afterward; the component and outside code reach ports as fields,
`r.Ports.Top`.

### Defining Components in V5: Philosophy and Patterns

V5 unifies how components are modeled and wired. Each component type is five structs — Spec, State, Resources, Ports, and Middlewares — and a `Definition`, declared in a package of its own; a component model turns them into a running component. The goals are: declarative configuration, local and serializable runtime state, explicit wiring, testability, and deterministic snapshot/restore.

#### Core Principles

1. Spec (immutable configuration)
   - Describes behavior and dependencies using only scalar fields: bool, numbers, strings, and named types based on them (such as `timing.Freq` or an enum-like `type Mode string`). No slices, arrays, maps, or nested structs.
   - Strategy dependencies are expressed as flat scalar fields: a kind plus its scalar parameters (e.g., `AddressMapperType: "interleaved"` and `InterleavingSize: 4096`).
   - No pointers or live objects in Spec. Keep it JSON/YAML‑friendly and hashable.
   - Defaults live in `Definition.DefaultSpec`; the system builder copies it, changes fields, and passes the result to `WithSpec`.

2. State (mutable runtime data)
   - Pure data only: scalars, slices, arrays, maps with string or integer keys, and simple nested structs.
   - No live handles, functions, channels, or ports in State.
   - All cross‑references use stable identifiers (IDs), never in‑memory pointers.
   - Written only by the component's own code: its `NewState`, its middlewares, and functions its package exports for the system builder (such as `ping.SchedulePing`).
   - A checkpoint serializes the State; loading one replaces it wholesale.

3. Resources (references supplied by the system builder)
   - References to shared objects — a `*mem.Storage`, a `vm.PageTable`, a `mem.AddressToPortMapper` — passed with `WithResources`.
   - Not checkpointed. A shared object that holds data checkpoints itself as a registered resource, and the rebuild supplies the same references again.

4. Ports (declared by the component, created by the system builder)
   - A component declares the ports it has as the fields of its `Ports` struct, tagged with the protocol roles they speak, but never constructs the instances or owns connections.
   - The system builder creates each port with `messaging.NewPort("<instance>.<Field>", in, out)`, choosing its buffer sizes, and passes them all to `Build`, which binds them to the component and registers them with the simulation.
   - Middlewares reach ports as fields (`m.comp.Ports.Top`), checked by the compiler.

5. Middlewares (ordered, holding only references)
   - Each middleware implements `Handle(e timing.Event) bool` and is an exported pointer field of `Middlewares`; the component passes every event to them in field order. Created in `NewMiddlewares`.
   - A middleware holds the component pointer and nothing mutable; everything that changes lives in State.
   - In a ticking component, prefer tick‑driven countdowns/backpressure over ad‑hoc scheduled events for simpler snapshots and determinism. When the behavior is naturally a set of timed events, use the event model instead.

#### Dependency Injection and Shared State

- Strategy injection (e.g., address conversion)
  - Keep in Spec as flat scalar fields (a `Kind` string plus scalar parameters), not as a live object.
  - Resolve to concrete implementations in the component package (in `NewState` or `NewMiddlewares`), or accept a ready-made object through Resources — the caches take a `mem.AddressToPortMapper` either way.
  - On restore, reconstruct from Spec; never serialize strategy objects.

- Emulation state (e.g., memory storage)
  - Treat as shared state separate from timing logic. The system builder creates the `*mem.Storage` and passes it through Resources (`dram.Resources{Storage: storage}`); several components may share one.
  - The storage checkpoints itself once, as a registered resource; components checkpoint only their own State.

#### Build and Wire

1. Create ports
   - Create every port with `messaging.NewPort`, named `<instance>.<Field>` (or `<instance>.<Field>[i]` for member `i` of a port group).
   - Creating ports first lets one component's Spec name another's port (`spec.BottomUnit = port.AsRemote()`).

2. Build from the Definition
   - `Definition.Builder().WithSimulation(sim).WithSpec(spec).WithResources(res).WithPorts(ports).Build(name)` validates the configuration, binds the ports, creates the State and middlewares, and registers everything with the simulation.

3. Wire topology
   - Build the connections and plug in the ports, reached as `comp.Ports.X`.
   - Use names consistently so components and tooling can introspect topology.

#### Determinism and Introspection

- Determinism: avoid non‑deterministic IDs or iteration order; snapshot ID generators; canonicalize map iteration by sorting.
- Introspection: `comp.Spec()` returns the effective Spec and `comp.State` can be dumped for debugging; the `inspect` package reads each `Definition`, its ports, and their roles without running the code.
- Tracing/metrics: attach as hooks; avoid embedding tracing in business logic.

#### Testing and Mocks

- Favor local interfaces inside the component package to reduce external coupling (e.g., `Storage`, `AddressConverter`, `StateAccessor`).
- Generate mocks from local interfaces for unit tests; avoid importing remote mocks.
- Drive behavior via ticks and ports; avoid requiring real engines or networks in unit tests. `modelingtest.Tick(comp)` hands every middleware of a ticking component one tick and reports whether any made progress, without scheduling the next one, so a test steps the component cycle by cycle.
- Add a `modelingtest.CheckTicking(t, Definition)` test (or `CheckWakeup`, `CheckEvent`) to every component package.

#### Example: Ideal Memory Controller (V5)

- Spec
  - `Freq`, `Width`, `Latency`, `CacheLineSize`; defaults in `Definition.DefaultSpec`.

- State
  - Pure data transactions with countdowns (`InflightTransactions`); no ports or live pointers.
  - Control mode (enabled, paused, draining) as a small enum, plus the command being served.

- Resources
  - `Storage *mem.Storage`, required; the system builder sizes it and may share it.

- Ports
  - `Top` (`role=github.com/sarchlab/akita/v5/mem/memprotocol.responder`) and `Control` (`role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.responder`), created by the system builder.

- Middlewares
  - `Ctrl` runs first: processes enable/pause/drain/reset; replies only when safe (e.g., after drain completes).
  - `Memory` is the data path: tick‑driven; takes requests from `Top`, counts down latency, reads or writes `Storage`, responds when ready.

This pattern generalizes to other components: keep Spec flat and declarative, keep State pure and serializable, take shared objects through Resources and ports through `Ports`, and implement behavior as ordered middlewares with minimal, explicit dependencies.

### Moving Container Fields Out of Spec

V5 Spec fields must be scalars. `Build` panics if a Spec has a slice, array, map, nested struct, pointer, or interface field, and the `inspect` package reports the same error. A V4 configuration field that holds a list is usually one of three things:

| The list is… | Move it to | Example |
|---|---|---|
| One value repeated per unit | A single scalar in Spec | A per-SIMD `VGPRCounts []int` whose entries are all equal becomes `VGPRPerSIMD int`. |
| Wiring, or derived only from wiring | Resources, used directly by the component | The caches route through the `mem.AddressToPortMapper` in Resources instead of a list of remote port names. |
| Runtime data that changes while simulating | State | Queues, in-flight transaction tables. |

Resources are not checkpointed. The setup that rebuilds a simulation supplies them again, so a restored component uses the rebuilt wiring. Do not copy wiring into State: `LoadCheckpoint` replaces the State wholesale and would bring back the wiring of the saved run.

### Migration Checklist

- Give each component type its own package, and choose its model: `modeling/ticking` for cycle-by-cycle work, `modeling/wakeup` or `modeling/event` for clockless components.
- Split each component's configuration into Spec (scalar user settings, including `Freq timing.Freq` for a ticking component), State (data that changes while simulating), and Resources (references to external objects and the wiring derived from them).
- Replace every slice, array, map, or nested-struct Spec field using the table above.
- Express strategy choices as a named string type with constants plus scalar parameters, not as a nested sub-spec.
- Give every Spec and State field a `json` tag, and make sure no two fields share a JSON name.
- Replace port fields, port declarations, and port lookups by name with a `Ports` struct: one `messaging.Port` field per port, `[]messaging.Port` per port group, each tagged `akita:"role=<protocol>.<role>"` when it speaks a protocol role, where `<protocol>` is the import path of the package that defines the protocol. Reach ports as `comp.Ports.X`.
- Turn each per-tick method into `Handle(e timing.Event) bool`, list the middlewares as exported pointer fields of a `Middlewares` struct in the order they run, and create them in a `newMiddlewares(c *Comp) Middlewares` function. Move any mutable middleware field into State.
- Declare `type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]` and `var Definition = ticking.Definition[...]{DefaultSpec: ..., NewState: ..., NewMiddlewares: ...}`, and delete the hand-written builder and constructor.
- Turn exported methods on the component into package functions that take `*Comp`.
- In the system builder, create every port with `messaging.NewPort("<instance>.<Field>", in, out)` and build with `Definition.Builder().WithSimulation(sim).WithSpec(spec).WithResources(res).WithPorts(ports).Build(name)`; start a component that begins work on its own with `TickLater()`.
- In tests, step the component with `modelingtest.Tick(comp)` and add a `modelingtest.CheckTicking(t, Definition)` test.

---

## 7. DRAM Improvements

V5 introduces a cycle-accurate DRAM memory controller with bank-state
machine, proper command sequencing, and timing constraints modeled after
DRAMsim3.

### Bank-State Machine

Each bank tracks its state (`Open`, `Closed`, `SRef`, `PD`) and the
currently executing command. Command kinds:

```go
// v5/mem/dram/spec.go
const (
    CmdKindRead CommandKind = iota
    CmdKindReadPrecharge
    CmdKindWrite
    CmdKindWritePrecharge
    CmdKindActivate
    CmdKindPrecharge
    CmdKindRefreshBank
    CmdKindRefresh
    CmdKindSRefEnter
    CmdKindSRefExit
)
```

### Timing Constraints

The DRAM controller enforces minimum cycle gaps between commands using a
`TimeTable` that encodes constraints at four scopes: same bank, other
banks in the same bank group, same rank, and other ranks.

Key timing parameters (all in cycles):

| Parameter | Description |
|-----------|-------------|
| `tRCD` | Row-to-Column Delay (ACT → READ/WRITE) |
| `tRAS` | Row Active Strobe (ACT → PRE minimum) |
| `tRP` | Row Precharge (PRE → ACT) |
| `tCCDL` / `tCCDS` | Column-to-Column Delay (long/short, same/diff bank group) |
| `tRRDL` / `tRRDS` | Row-to-Row Delay (ACT → ACT across banks) |
| `tFAW` | Four-Activation Window |
| `tWR` | Write Recovery (last write data → PRE) |
| `tWTRL` / `tWTRS` | Write-to-Read turnaround (long/short) |
| `tRTP` | Read-to-Precharge |
| `tRFC` | Refresh Cycle time |
| `tREFI` | Refresh Interval |

### Presets

V5 ships with validated presets for common DRAM technologies:

```go
// v5/mem/dram/presets.go
dram.DDR4Spec   // DDR4-2400 (1200 MHz, BL8, 4 bank groups × 4 banks)
dram.DDR5Spec   // DDR5-4800 (2400 MHz, BL16, 8 bank groups × 4 banks)
dram.HBM2Spec   // HBM2-2Gbps (1000 MHz, BL4, 4 bank groups × 4 banks, 128-bit bus)
dram.HBM3Spec   // HBM3-6.4Gbps (3200 MHz, BL8)
dram.GDDR6Spec  // GDDR6-14Gbps (1750 MHz, BL16)
```

### Usage

A preset is a complete `dram.Spec`, including its `Freq`; copy it, change the
fields you need, and pass it to `WithSpec`:

```go
spec := dram.DDR4Spec
spec.PagePolicy = dram.PagePolicyOpen

storage := mem.NewStorage(4 * mem.GB)

ctrl := dram.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(dram.Resources{Storage: storage}).
    WithPorts(dram.Ports{
        Top:     messaging.NewPort("DRAM.Top", 1024, 1024),
        Control: messaging.NewPort("DRAM.Control", 4, 4),
    }).
    Build("DRAM")
```

### Statistics

The DRAM state tracks runtime statistics:

```go
state := ctrl.State

// Latency
avgRead := dram.AverageReadLatency(&state)   // cycles
avgWrite := dram.AverageWriteLatency(&state) // cycles

// Bandwidth
readBW := dram.ReadBandwidth(&state)   // bytes per cycle
writeBW := dram.WriteBandwidth(&state) // bytes per cycle

// Row buffer
hitRate := dram.RowBufferHitRate(&state) // 0.0 to 1.0

// Raw counters available in state:
// state.TotalReadCommands, state.TotalWriteCommands,
// state.TotalActivates, state.TotalPrecharges,
// state.RowBufferHits, state.RowBufferMisses,
// state.BytesRead, state.BytesWritten, etc.
```

### Before / After

**Before (V4) — idealized memory controller:**
```go
// V4: Fixed-latency memory controller, no bank modeling.
ctrl := idealmemcontroller.New().
    WithSimulation(sim).
    WithFreq(1 * sim.GHz).
    WithLatency(100).
    Build("MemCtrl")
```

**After (V5) — cycle-accurate DRAM:**
```go
// V5: Bank-state machine with proper command sequencing.
spec := dram.HBM2Spec
spec.PagePolicy = dram.PagePolicyOpen

ctrl := dram.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(dram.Resources{Storage: storage}).
    WithPorts(dram.Ports{
        Top:     messaging.NewPort("DRAM.Top", 4, 4),
        Control: messaging.NewPort("DRAM.Control", 4, 4),
    }).
    Build("DRAM")
```

---

## 8. Port Creation API

In V4, ports were created internally by component builders. In V5, the
component owns its port *topology* — the fields of its `Ports` struct say
which ports it has — but it does not create the instances. The system builder
creates each port with `messaging.NewPort` and passes all of them to `Build`
through `WithPorts`. This makes wiring explicit and lets ports be sized or
implemented differently without changing the component.

**Before (V4):**
```go
// V4: Builder creates ports internally — caller has no control over port creation.
cache := cachebuilder.New().
    WithSimulation(sim).
    WithFreq(1 * sim.GHz).
    Build("Cache")
```

**After (V5):**
```go
// V5: the component declares its ports as the fields of its Ports struct;
// the system builder creates each port and passes them all to Build.
cache := writeback.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(writeback.Resources{Storage: storage}).
    WithPorts(writeback.Ports{
        Top:     messaging.NewPort("Cache.Top", 4, 4),
        Bottom:  messaging.NewPort("Cache.Bottom", 4, 4),
        Control: messaging.NewPort("Cache.Control", 4, 4),
    }).
    Build("Cache")

conn.PlugIn(cache.Ports.Top)
```

`Build` binds each port to the new component and registers it with the
simulation, exactly as it registers the component. It panics if a port is
missing, is not named `<instance>.<Field>`, or already belongs to another
component, so a typo or a forgotten port fails fast. No port is added after
`Build`.

A port group is a `[]messaging.Port` field; the system builder chooses its
size and names member `i` `<instance>.<Field>[i]`.

### SetOwner

The `Port` interface in V5 includes a `SetOwner(owner PortOwner)` method.
Because the system builder creates ports before the component exists, a port
is created without an owner, and `Build` calls `SetOwner` to associate
it with the component:

```go
outPort := messaging.NewPort("Agent.Out", 4, 4)

agent := ping.Definition.Builder().
    WithSimulation(sim).
    WithPorts(ping.Ports{Out: outPort}).
    Build("Agent") // calls outPort.SetOwner(agent)
```

Creating the port first also lets another component's Spec name it
(`outPort.AsRemote()`) before either component is built. This decouples port
creation from component construction, which suits the V5 wiring model where
topology is assembled separately from component internals.

---

## 9. Queueing

The `queueing` package provides generic buffer and pipeline implementations that follow V5 design principles. V4's interface/implementation pattern (`sim.Buffer`, `pipelining.Pipeline`) is replaced by direct generic struct literals — no constructors, no builders, no pointer indirection.

### Key Changes from V4 to V5

**V4 Pattern (Interface + Constructor):**
```go
// V4: Interface abstraction with hidden implementation. Pipelines were
// likewise a pipelining.Pipeline interface, created by the package's builder.
var buffer sim.Buffer = sim.NewBuffer("name", 10)
```

**V5 Pattern (Generic Struct Literals):**
```go
// V5: Direct struct literal, no constructors or interfaces
buffer := queueing.Buffer[int]{BufferName: "name", Cap: 10}
pipeline := queueing.Pipeline[int]{NumStages: 5, Width: 1}
```

### Migration Benefits

1. **Compile-time Type Safety**: Generic type parameter `[T]` ensures buffers and pipelines are type-safe at compile time.
2. **JSON-Serializable State**: All fields have `json` tags, making them compatible with V5's state serialization requirements.
3. **Value Types**: Buffers and pipelines are value types (no pointers), following V5's no-pointers-in-State rule.
4. **Simplified APIs**: No constructors, builders, or interface abstractions — just struct literals.
5. **Maintained Functionality**: All essential features preserved including hook support (`sim.HookableBase`), FIFO queue behavior, and multi-stage pipeline processing.

### Usage Examples

**Buffer Migration:**
```go
// V4
buffer := sim.NewBuffer("MyBuffer", 100)

// V5
buffer := queueing.Buffer[int]{BufferName: "MyBuffer", Cap: 100}
```

**Pipeline Migration:**

V4 built a pipeline through the `pipelining` package's builder, setting the
number of stages (`WithNumStage`), the cycles per stage (`WithCyclePerStage`),
and a post-pipeline buffer (`WithPostPipelineBuffer`).

```go
// V5 — Pipeline has Width, NumStages, and Stages fields only.
// No CyclePerStage or Name fields.
pipeline := queueing.Pipeline[int]{NumStages: 5, Width: 1}
```

### V5 Component Integration

When building V5 components, use `queueing` value types (not pointers) with generic type parameters in your component State:

```go
type MyComponentState struct {
    InputBuffer  queueing.Buffer[int]   `json:"input_buffer"`
    Pipeline     queueing.Pipeline[int] `json:"pipeline"`
    OutputBuffer queueing.Buffer[int]   `json:"output_buffer"`
}
```

This aligns with V5 principles: `State` fields are value types with `json` tags, making them easily serializable and restorable without deep-copy complications.

---

## 10. CLI Changes

- Command rename: `akita check [path]` → `akita component-lint [path]`.
  - Usage: `akita component-lint .`, `akita component-lint ./...`, `akita component-lint -r mem/`
  - Directories without `//akita:component` are reported as `not a component` and do not fail.
- New scaffolding: `akita component-create <path>` replaces `component --create`.
  - Example: `akita component-create mem/newcontroller`
  - Must run inside the Akita Git repository.

---

## 11. CI Migration

The CI pipeline now uses **self-hosted runners** instead of GitHub-hosted runners. All jobs in `.github/workflows/akita_test.yml` specify `runs-on: self-hosted`.

Key points:
- All workflow jobs (compile, lint, unit test, acceptance tests) run on self-hosted infrastructure.
- The workflow triggers on both `push` and `pull_request` events.
- Mock generation (`go generate ./...`) and builds operate from the `v5/` directory.
- No changes to test commands are needed — only the runner target has changed.
