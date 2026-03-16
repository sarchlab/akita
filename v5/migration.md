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
6. [Component Model: Spec + State + Ports + Middleware + Hooks](#6-component-model)
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
| `type VTimeInSec float64` (seconds) | `type VTimeInSec uint64` (picoseconds) |
| `type Freq float64` (cycles/second) | `type Freq uint64` (Hz) |

Despite the legacy name `VTimeInSec`, the unit is now **picoseconds**.

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
| `Period()` | `func (f Freq) Period() VTimeInSec` | Picoseconds per cycle. 1 GHz → 1000 ps. |
| `Cycle()` | `func (f Freq) Cycle(time VTimeInSec) uint64` | Number of complete cycles since time 0. |
| `ThisTick()` | `func (f Freq) ThisTick(now VTimeInSec) VTimeInSec` | Ceil to nearest tick boundary. |
| `NextTick()` | `func (f Freq) NextTick(now VTimeInSec) VTimeInSec` | Next tick strictly after `now`. |
| `NCyclesLater()` | `func (f Freq) NCyclesLater(n int, now VTimeInSec) VTimeInSec` | Time `n` cycles from current tick. |
| `HalfTick()` | `func (f Freq) HalfTick(t VTimeInSec) VTimeInSec` | Midpoint between two ticks. |

### Before / After

**Before (V4) — float64 seconds:**
```go
freq := sim.Freq(1e9) // 1 GHz as float64

now := sim.VTimeInSec(0.000000001) // 1 nanosecond
period := 1.0 / float64(freq)      // 1e-9 seconds
next := now + sim.VTimeInSec(period)

// Danger: float rounding errors accumulate over millions of cycles.
```

**After (V5) — uint64 picoseconds:**
```go
freq := 1 * sim.GHz // Freq(1_000_000_000)

now := sim.VTimeInSec(1000) // 1000 ps = 1 ns
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

```go
// v5/sim/idgenerator.go
type IDGenerator interface {
    Generate() uint64
}

// Two implementations: sequential (deterministic) and parallel (non-deterministic).
sim.UseSequentialIDGenerator() // Call before any Generate()
sim.UseParallelIDGenerator()   // For parallel simulations

id := sim.GetIDGenerator().Generate() // returns uint64
```

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
req.ID = sim.GetIDGenerator().Generate() // 1, 2, 3, ...
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
- Checkpoint/restore: use `GetIDGeneratorNextID()` / `SetIDGeneratorNextID()` to snapshot generator state.

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
    Time() VTimeInSec
    HandlerID() string    // was Handler() Handler in V4
    IsSecondary() bool
}

type EventBase struct {
    ID         uint64     `json:"id"`
    Time_      VTimeInSec `json:"time"`
    HandlerID_ string     `json:"handler_id"`
    Secondary  bool       `json:"secondary"`
}
```

### Handler Registration

The engine implements `HandlerRegistrar`:

```go
// v5/sim/engine.go
type HandlerRegistrar interface {
    RegisterHandler(name string, handler Handler)
}
```

Components register themselves during construction. For example,
`NewTickingComponent` automatically registers with the engine:

```go
// v5/sim/ticker.go
func NewTickingComponent(
    name string,
    engine EventScheduler,
    freq Freq,
    ticker Ticker,
) *TickingComponent {
    tc := new(TickingComponent)
    tc.TickScheduler = NewTickScheduler(name, engine, freq)
    tc.ComponentBase = NewComponentBase(name)
    tc.ticker = ticker

    // Auto-register so events with HandlerID_==name route here.
    if registrar, ok := engine.(HandlerRegistrar); ok {
        registrar.RegisterHandler(name, tc)
    }

    return tc
}
```

### Before / After

**Before (V4) — interface reference:**
```go
type Event interface {
    Time() VTimeInSec
    Handler() Handler    // direct pointer to handler
    IsSecondary() bool
}

// Creating an event:
evt := sim.NewEventBase(now, myComponent) // passed handler object
```

**After (V5) — string ID + registry:**
```go
type Event interface {
    Time() VTimeInSec
    HandlerID() string   // string name, serializable
    IsSecondary() bool
}

// Creating an event:
evt := sim.NewEventBase(now, "MyComponent") // pass handler name
```

### Migration Checklist

- Replace `evt.Handler()` calls with `evt.HandlerID()`.
- Replace `NewEventBase(time, handlerObj)` with `NewEventBase(time, "handlerName")`.
- Ensure all event handlers are registered with the engine via `RegisterHandler`.
- Custom event types: change the `Handler` field from interface to `string`.

---

## 5. In-Place State Update

**Motivation:** V4 used a double-buffered model where `current` and `next`
were deep copies. This was expensive and error-prone. V5 simplifies to
in-place updates where `current` and `next` refer to the same data during
a tick.

### V5 Tick Cycle

From `v5/modeling/component.go`:

```go
func (c *Component[S, T]) Tick() bool {
    c.next = c.current         // 1. Assign current to next (shallow copy)
    madeProgress := c.MiddlewareHolder.Tick()  // 2. Middlewares modify next
    c.current = c.next         // 3. Promote next back to current
    return madeProgress
}
```

Because both `current` and `next` are the **same value** (shallow copy of
a struct), middlewares can read from `GetState()` or `GetNextState()`
interchangeably. Mutations via `GetNextState()` are immediately visible
through `GetState()`.

### State Access

```go
// Read current state (same data as next during a tick).
state := comp.GetState()

// Get pointer to next state for mutation.
next := comp.GetNextState()
next.Counter++

// SetState sets both current and next (for init / checkpoint restore).
comp.SetState(initialState)
```

### Before / After

**Before (V4) — deep copy double buffer:**
```go
// V4: current and next were separate deep copies.
// Reading current gave the pre-tick snapshot.
// Writing next didn't affect current until commit.
state := comp.GetState()     // snapshot from start of tick
next := comp.GetNextState()  // separate copy
next.Value = state.Value + 1 // must read from state, write to next
comp.CommitNextState()        // deep copy next → current
```

**After (V5) — in-place update:**
```go
// V5: current and next are the same data.
// Read and write through GetNextState pointer.
next := comp.GetNextState()
next.Value++                  // direct mutation, visible immediately

// No explicit commit needed — Tick() handles it.
```

### Migration Checklist

- Remove any deep-copy logic between current/next state.
- `GetState()` and `GetNextState()` return the same underlying data during a tick — choose one and be consistent.
- For initialization, use `comp.SetState(initialState)` to set both current and next.
- For checkpoint restore, call `SetState()` followed by `ResetAndRestartTick()`.

---

## 6. Component Model

V5 unifies component structure into five orthogonal parts. See the
"Defining Components in V5" section below for the full philosophy.

### Anatomy

| Part | Role | Key Rule |
|------|------|----------|
| **Spec** | Immutable configuration | Primitives only. JSON-friendly. No pointers. |
| **State** | Mutable runtime data | Pure data. No ports, functions, channels. Use IDs for cross-references. |
| **Ports** | Communication endpoints | Created externally, injected via `AddPort(name, port)`. Never constructed internally. |
| **Middlewares** | Per-tick behavior pipeline | Ordered. Operate on State via `GetNextState()`. Stateless w.r.t. external deps. |
| **Hooks** | Observation/tracing | Attached via `HookableBase`. Don't affect simulation logic. |

### Generic Component

```go
// v5/modeling/component.go
type Component[S any, T any] struct {
    *sim.TickingComponent
    sim.MiddlewareHolder

    spec    S
    current T
    next    T
}
```

`S` is the Spec type, `T` is the State type. Both are plain structs.

### Building a Component

```go
comp := modeling.NewBuilder[MySpec, MyState]().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithSpec(mySpec).
    Build("MyComponent")

comp.SetState(initialState)
comp.AddMiddleware(&myMiddleware{comp: comp})

port := sim.NewPort(nil, 4, 4, "MyComponent.Top")
port.SetComponent(comp)
comp.AddPort("Top", port)
```

### Defining Components in V5: Philosophy and Patterns

V5 unifies how components are modeled and wired. Each component is a single struct composed of four orthogonal parts: Spec, State, Ports, and Middlewares. The goals are: declarative configuration, local and serializable runtime state, explicit wiring, testability, and deterministic snapshot/restore.

#### Core Principles

1. Spec (immutable configuration)
   - Describes behavior and dependencies using only primitives (bool, number, string) and primitive maps/slices.
   - Strategy dependencies are expressed as small primitive "sub‑specs" (e.g., `{ Kind: "interleaving", Params: { ... } }`).
   - No pointers or live objects in Spec. Keep it JSON/YAML‑friendly and hashable.
   - Validation and defaults are part of the component package (e.g., `validate()` + `defaults()`).

2. State (mutable runtime data)
   - Pure data only: scalars and slices/maps of primitives or simple structs thereof.
   - No live handles, functions, channels, or ports in State.
   - All cross‑references use stable identifiers (IDs), never in‑memory pointers.
   - Snapshot/restore uses deep copies of State so checkpoints are immutable.

3. Ports (externally injected)
   - Components never construct or own connections. Ports are created/injected during wiring and registered via `AddPort(name, port)`.
   - Components access ports by name via `GetPortByName("...")` to avoid compile‑time coupling.

4. Middlewares (ordered, stateless over the component)
   - Implement the per‑tick pipeline. Each middleware operates on the component's State and interacts with Ports.
   - Keep middlewares stateless wrt external dependencies; resolve them at build time and pass the resolved handles in.
   - Prefer tick‑driven countdowns/backpressure over ad‑hoc scheduled events for simpler snapshots and determinism.

#### Dependency Injection and Shared State

- Strategy injection (e.g., address conversion)
  - Keep in Spec as a primitive descriptor (`Kind`, `Params`), not as a live object.
  - Resolve to concrete implementations locally in the component builder and inject into middlewares.
  - On restore, reconstruct from Spec; never serialize strategy objects.

- Emulation state (e.g., memory storage)
  - Treat as shared state separate from timing logic. Store only an ID (e.g., `StorageRef`) in Spec/State.
  - Keep a per‑simulation state registry; components resolve handles by ID at runtime.
  - Snapshot/restore orchestrates shared state once per ID (outside components); components snapshot only their own State.

#### Build and Wire (two stages)

1. Build from Spec
   - `Builder.WithSpec(spec).WithEngine(engine).WithFreq(freq).Build(name)` constructs the component with defaults and resolved strategies.
   - Do not create or connect ports here.

2. Wire topology
   - Create ports and connections, then inject ports via `AddPort("...", port)`.
   - Use names consistently so components and tooling can introspect topology.

#### Determinism and Introspection

- Determinism: avoid non‑deterministic IDs or iteration order; snapshot ID generators; canonicalize map iteration by sorting.
- Introspection: provide methods to inspect effective Spec (with defaults) and to dump State for debugging.
- Tracing/metrics: attach as middlewares or hooks; avoid embedding tracing in business logic.

#### Testing and Mocks

- Favor local interfaces inside the component package to reduce external coupling (e.g., `Storage`, `AddressConverter`, `StateAccessor`).
- Generate mocks from local interfaces for unit tests; avoid importing remote mocks.
- Drive behavior via ticks and ports; avoid requiring real engines or networks in unit tests.

#### Example: Ideal Memory Controller (V5)

- Spec
  - Timing: `Width`, `LatencyCycles`, `Freq`.
  - Shared emulation: `StorageRef` (ID in simulation state registry).
  - Strategy: `AddrConv` as `{ Kind, Params }` (e.g., identity/interleaving).

- State
  - Pure data transactions with countdowns; no ports or live pointers.
  - Drain/enable mode as a small enum; deep‑copied for snapshots.

- Ports
  - `Top`, `Control` injected during wiring; accessed via name lookups.

- Middlewares
  - Data path: tick‑driven; consumes from `Top`, counts down latency, responds when ready; uses storage resolved via state registry by `StorageRef`.
  - Control path: processes enable/pause/drain; replies only when safe (e.g., after drain completes).

This pattern generalizes to other components: keep Spec primitive and declarative, keep State pure and serializable, inject Ports, and implement behavior as pipelines of middlewares with minimal, explicit dependencies.

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

```go
topPort := sim.NewPort(nil, 4, 4, "DRAM.Top")

ctrl := dram.MakeBuilder().
    WithEngine(engine).
    WithSpec(dram.DDR4Spec).
    WithFreq(1200 * sim.MHz).
    WithTopPort(topPort).
    Build("DRAM")
```

### Statistics

The DRAM state tracks runtime statistics:

```go
state := ctrl.GetState()

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
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithLatency(100).
    Build("MemCtrl")
```

**After (V5) — cycle-accurate DRAM:**
```go
// V5: Bank-state machine with proper command sequencing.
topPort := sim.NewPort(nil, 4, 4, "DRAM.Top")

ctrl := dram.MakeBuilder().
    WithEngine(engine).
    WithSpec(dram.HBM2Spec).
    WithTopPort(topPort).
    WithPagePolicy(dram.PagePolicyOpen).
    Build("DRAM")
```

---

## 8. Port Creation API

In V4, ports were created internally by component builders. In V5, ports are created externally and passed into builders via `WithXxxPort()` methods. This makes wiring explicit and allows ports to be shared or configured before a component is built.

**Before (V4):**
```go
// V4: Builder creates ports internally — caller has no control over port creation.
cache := cachebuilder.New().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Cache")
```

**After (V5):**
```go
// V5: Ports are created externally and injected into the builder.
topPort := sim.NewPort(nil, 4, 4, "Cache.TopPort")
bottomPort := sim.NewPort(nil, 4, 4, "Cache.BottomPort")
controlPort := sim.NewPort(nil, 4, 4, "Cache.ControlPort")

cache := cachebuilder.New().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithTopPort(topPort).
    WithBottomPort(bottomPort).
    WithControlPort(controlPort).
    Build("Cache")
```

This pattern applies to all V5 components. Each builder exposes `WithXxxPort(port sim.Port)` methods for every port the component needs.

### SetComponent

The `Port` interface in V5 now includes a `SetComponent(comp Component)` method. This allows creating a port before the owning component is built, then associating the port with the component afterward.

```go
// Create port before the component exists.
outPort := sim.NewPort(nil, 4, 4, "Agent.OutPort")

// Build the component, injecting the port.
agent := pingbuilder.New().
    WithOutPort(outPort).
    Build("Agent")

// The builder calls SetComponent internally, but you can also call it manually:
outPort.SetComponent(agent)
```

This decouples port creation from component construction, which is essential for the V5 wiring model where topology is assembled separately from component internals.

---

## 9. Queueing

The `queueing` package provides generic buffer and pipeline implementations that follow V5 design principles. V4's interface/implementation pattern (`sim.Buffer`, `pipelining.Pipeline`) is replaced by direct generic struct literals — no constructors, no builders, no pointer indirection.

### Key Changes from V4 to V5

**V4 Pattern (Interface + Constructor):**
```go
// V4: Interface abstraction with hidden implementation
var buffer sim.Buffer = sim.NewBuffer("name", 10)
var pipeline pipelining.Pipeline = pipelining.MakeBuilder().Build("name")
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
```go
// V4
pipeline := pipelining.MakeBuilder().
    WithNumStage(5).
    WithCyclePerStage(2).
    WithPostPipelineBuffer(postBuf).
    Build("MyPipeline")

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

This aligns with V5 principles: State fields are value types with `json` tags, making them easily serializable and restorable without deep-copy complications.

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
