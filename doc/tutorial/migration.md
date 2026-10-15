---
sidebar_position: 7
---

# V4 → V5 Migration Guide

This guide covers all breaking changes between Akita V4 and V5. Each section
explains the motivation, shows before/after code, and notes pitfalls.

## Construction, binding, and initialization

Configure plain Spec structs first; changing a Spec before Build changes the
next instance. Build components with their final full names and Resources,
then bind ports and connections. Finally initialize the shared simulation:

```go
spec := rob.Definition.DefaultSpec
spec.BufferSize = 8
r := rob.Definition.Builder().WithSimulation(s).WithSpec(spec).Build("GPU.ROB")
r.BindPort("Top", twowaybuffered.NewPort(8, 8))
r.BindPort("Bottom", twowaybuffered.NewPort(8, 8))
r.BindPort("Control", twowaybuffered.NewPort(1, 1))
conn := direct.NewConnection("GPU.Link", s, timing.GHz)
conn.BindPort(r.Ports.Top)
// Bind the other components and links before initialization.
if err := s.Initialize(); err != nil {
    panic(err)
}
// State and Middlewares now exist. Seed work, then run.
```

`Build` registers the configured component and event handler. `BindPort` fills
the declared slot, assigns the owner and name (`GPU.ROB.Top`), and registers
the port. Indexed groups use names such as `BindPort("Inputs[0]", p)`.
`connection.BindPort(p)` establishes both sides of the connection attachment.
Bindings are one-time; rejected bindings leave existing wiring intact.

`Initialize` checks every declared port before running any initializer.
Missing ports and slice holes return an error and leave setup editable. After
validation succeeds, topology freezes and `State.Initialize`/`Middlewares.Initialize` run in
component registration order. An initializer panic leaves that simulation
unusable; rebuild it instead of retrying partially created runtime state.
Initialize once before execution, checkpoint save/load, or seeding work that
uses State or Middlewares. Rebuild and initialize before restoring a checkpoint.

`AsRemote()` requires a bound owner. During configuration, use the planned full
port name, or build and bind the destination before asking for its address.
Components remain unaware of domains; domain configuration/builders are a
separate change.

Use `sim.MakeBuilder().Build()` for every setup; StandaloneSimulation is removed.
`Simulation.Run()` checks that Initialize succeeded. Direct `Engine.Run()` and
`SerialEngine.RunUntil(t)` require prior initialization but do not check it.
RunUntil remains only on SerialEngine for deterministic checkpoint boundaries.
Custom engine injection is deferred to v5.2.

Recording is off by default. `WithOutputPath(path)` enables it at an exact path
and rejects existing files. `WithVisTracingOnStart` and `WithMonitor` also enable
recording, choosing a unique filename when none is supplied. `WithTaskTracking`
alone works in memory; `VisTracer()` is nil. For manual database tracing, combine
tracking with an explicit recording option.

`datarecording.NewDataRecorder(path)` also uses the exact path. Previously,
`NewDataRecorder("trace")` appended `.sqlite3`; now it creates `trace`. Pass
`"trace.sqlite3"` explicitly to retain that filename. Existing files are rejected.

Memory routing now uses `mem.AddressMapper` values in Spec fields named after
the routed port: `Bottom`, `Translation`, `Inside`, or `Outside`. The fixed
peer fields `gmmu.Spec.LowModule` and `rob.Spec.BottomUnit` are also now `Bottom`.
`SinglePortMapper`, `InterleavedAddressPortMapper`, `BankedAddressPortMapper`,
and `AddressMapperType` are removed; `AddressToPortMapper` remains.

```go
spec.Bottom = mem.AddressMapper{
    Destinations: []messaging.RemotePort{"DRAM0.Top", "DRAM1.Top"},
    InterleavingSize: 256,
    RangeLow: 1024, // inclusive
    RangeHigh: 4096, // exclusive; zero disables range filtering
    Fallback: "OtherMemory.Top",
}
```

A single destination receives every in-range address. Multiple destinations
repeat every `InterleavingSize` bytes using the absolute address, without
translating it. Component initialization requires at least one nonempty destination and a positive
interleaving size when there are multiple destinations. A configured range must
have `RangeHigh > RangeLow` and a nonempty `Fallback`; addresses outside it use
that fallback. With `RangeHigh == 0`, no fallback is required.

The former banked mapper panicked beyond its last bank; the unified mapper wraps
across destinations. To bound a mapping, configure the range and fallback.
Routing participates in the Spec hash; rebuild checkpoints after this API migration.

Endpoints and switches moved from
`noc/networking/switching/{endpoint,switches}` to
`noc/internal/{endpoint,switches}`.
Applications build NoCs through `networkconnector`, `mesh`, `pcie`, or `nvlink`.
Switch port names change from `Port[i]` to `Links[i]`, including in traces and
monitoring output.
Internally, endpoint device ports bind to `DeviceConnection(ep)`, and Initialize
checks their ordered addresses against `Spec.Devices`. The endpoint stays a
pass-through connection, preserving message sources and the existing hop count.

Monitor and NoC connector configuration now uses value builders:
`MakeBuilder().With...().Build()` (or `Build(name)` for a connector). Only builders
have With methods; topology assembly happens on the built connector. Use
`WithFreq` consistently. Frequency/bandwidth derived values are computed at Build.
Imports move from `noc/networking/...` to `noc/...`, and from
`mem/cache/writethroughcache` to `mem/cache/writethrough`.

## Domain API deferred to v5.1

The pre-release `modeling.Domain[P]` and `modeling.NewDomain` APIs have been
removed before v5.0. Domain Spec/builders and hierarchical composition support
are planned for v5.1 ([#498](https://github.com/sarchlab/akita/issues/498)).

For v5.0, use ordinary Go structs and builders to group components, assign their
final hierarchical names before construction, and expose existing member ports.
Exposing a port does not rebind its owner. Complete the platform's wiring before
calling `Initialize()`.

## Buffered ports and direct connections

Concrete ports and connections now live under `sim/messaging`:

| Before | After |
|---|---|
| `messaging.NewPort(name, in, out)` | `twowaybuffered.NewPort(in, out)` |
| `noc/directconnection` | `sim/messaging/direct` |
| `directconnection.Comp` | `direct.Connection` |
| `directconnection.MakeBuilder()...Build(name)` | `direct.NewConnection(name, s, freq)` |
| `directconnection.DefaultSpec()` | Pass the frequency directly; no public spec or defaults |

Import `github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered` for port
construction. Components continue to declare `messaging.Port` fields, while
`NewPort` now returns the concrete `*twowaybuffered.Port`. Old constructors and
import paths are removed. Calls that type-asserted the old constructor result
can now call checkpoint methods directly on the returned pointer.

Ports are now unnamed until their owner binds them. `SetOwner`, `SetConnection`,
`WithPorts`, and connection `PlugIn` are removed. Buffer behavior and message
checkpoint encoding are unchanged.

Direct connections use a constructor, just like ports. Pass an explicit frequency
(`timing.GHz` preserves the former default); the constructor registers the
connection and its event handler with the simulation. Neither ports nor direct
connections use the component `Definition` and builder API.

The NoC connector creates only buffered ports and ideal direct connections.
Remove `WithPortFactory` calls; `PortFactory` is removed. Device ports attached
to an endpoint must be `*twowaybuffered.Port`; unsupported implementations,
including nil ports, panic during device wiring before any device port is
attached. Wire support and non-ideal link timing remain separate changes.

## Simulation umbrella and tool names

The runtime packages now live below `sim/`: `naming`, `hooking`,
`queueing`, `timing`, `messaging`, `modeling`, `tasks`, `datarecording`, and
`sourcefs`.
Task observers live in `sim/tasks/tracing`; replace old `sim/tracing` imports
with `sim/tasks` for task commands and `sim/tasks/tracing` for observers.
Change imports such as `github.com/sarchlab/akita/v5/timing` to
`github.com/sarchlab/akita/v5/sim/timing`; apply the same change to
component-model subpackages and mock-generation directives. The runner also moves from
`github.com/sarchlab/akita/v5/simulation` to `github.com/sarchlab/akita/v5/sim`;
its Go package is now named `sim`, while the type remains `sim.Simulation`.

Source archives currently live in `sim/sourcefs`. Register a simulator's source
with `sim.Builder.WithSourceFS`; moving the archive implementation to `internal/`
is tracked separately in [#539](https://github.com/sarchlab/akita/issues/539).

The live monitor is now `github.com/sarchlab/akita/v5/monitoring` and the
visualizer is `github.com/sarchlab/akita/v5/daisen`. The old package paths are
removed. Browser watched-property storage keys are retained so the rename
preserves existing saved preferences.

**Monitoring default changed:** building a simulation no longer starts a web
server unless a monitor is supplied. Downstream simulators, including MGPUSim,
must add `WithMonitor` to retain live monitoring; importing the package alone
does not enable it. MGPUSim migration is a separate change.

```go
monitor := monitoring.MakeBuilder().WithPortNumber(8080).Build()
s := sim.MakeBuilder().WithMonitor(monitor).Build()
defer s.Terminate()
bar := monitor.CreateProgressBar("kernels", 100)
```

Remove `WithoutMonitoring()` calls; omitting `WithMonitor` disables monitoring.
Tracing remains independent. Move `WithMonitorPort(port)` configuration to
`monitoring.MakeBuilder().WithPortNumber(port).Build()`. Keep the concrete `monitor`
variable for progress bars and other monitor-specific operations.

Replace manual `RegisterSimulation`, `RegisterVisTracer`, `SetTraceDBPath`, and
`StartServer` calls with `WithMonitor`. The runner calls `Start(s)` during
`Build` to bind the simulation and start the server, then `Stop()` during termination. Component
State and Middlewares are created later by `Initialize`.
A monitor can be started only once, even after stopping, and progress bars must
be created after startup.

`daisen.ProgressBar` moves to `monitoring.ProgressBar`; the offline viewer no
longer owns live progress tracking. Component code should depend on the methods
it uses rather than a UI type. `memaccessagent.ProgressTracker` declares only
`IncrementInProgress` and `MoveInProgressToFinished`. Replace
`memaccessagent.CreateProgressBars` with explicitly supplied trackers:

```go
memaccessagent.SetProgressTrackers(agent,
    monitor.CreateProgressBar(agent.Name()+".Writes", uint64(agent.State.WriteLeft)),
    monitor.CreateProgressBar(agent.Name()+".Reads", uint64(agent.State.ReadLeft)),
)
```

Call this after initializing a fresh agent and before running. Either tracker can
be nil to disable reporting for that operation. After checkpoint restoration,
initialize fresh trackers for both remaining and already in-flight requests.

---

## Table of Contents

- [Simulation umbrella and tool names](#simulation-umbrella-and-tool-names)

1. [Integer Time](#1-integer-time)
2. [uint64 Entity IDs](#2-uint64-entity-ids)
3. [Unified Control Protocol](#3-unified-control-protocol)
4. [Event Serialization: Handler() → HandlerID()](#4-event-serialization-handler--handlerid)
5. [Single-State Update](#5-single-state-update)
6. [Component Models: Five Structs + Definition](#6-component-model)
7. [DRAM Improvements](#7-dram-improvements)
8. [Port Creation API](#8-port-creation-api)
9. [Queueing](#9-queueing)
10. [CLI Limitations](#10-cli-limitations)
11. [CI Validation](#11-ci-validation)

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
| `MsgMeta.ID string` | `Msg.ID uint64` |
| `MsgMeta.RspTo string` | `Msg.RspTo uint64` |
| `IDGenerator.Generate() string` | `IDGenerator.Generate() uint64` |
| `tracing.Task.ID string` | `tasks.Task.ID uint64` |
| `tracing.Task.ParentID string` | `tasks.Task.ParentID uint64` |
| Empty/nil sentinel: `""` | Empty/nil sentinel: `0` |

### Message envelopes and value payloads (V5)

Before (the earlier V5 API):

```go
type ReadReq struct {
    messaging.MsgMeta
    Address uint64
}
req := ReadReq{MsgMeta: messaging.MsgMeta{ID: id, Src: src, Dst: dst}, Address: 64}
port.Send(req)
switch req := msg.(type) {
case ReadReq:
    handle(req.Meta().ID, req.Address)
}
```

After:

```go
type ReadReq struct { Address uint64 }
var Protocol = messaging.DefineProtocol(
    messaging.RoleDef{Name: "requester", Sends: []any{ReadReq{}}},
)
req := messaging.Msg{ID: id, Src: src, Dst: dst, Payload: ReadReq{Address: 64}}
port.Send(req)
switch req := msg.Payload.(type) {
case ReadReq:
    handle(msg.ID, req.Address)
}
```

`Msg` contains `ID`, `Src`, `Dst`, `TrafficClass`, `TrafficBytes`, `RspTo`, and
`Payload`. `msg.IsRsp()` means `msg.RspTo != 0`. The earlier `MsgMeta` type and
`Meta()` method are removed. Tracing task IDs live in component transaction State and port buffer entries; see the tracing migration in [V5 release notes](https://github.com/sarchlab/akita/blob/main/doc/release-notes-v5.md#simulation-scoped-task-tracking).

Payloads must be registered values, including in examples that never checkpoint.
Pointer prototypes and pointer/interface/channel/function/unsafe-pointer fields
are rejected at registration, including JSON-excluded fields. Slice and map
storage remains shared; do not mutate it after sending. A nil payload is allowed.
Use message IDs for identity comparisons: comparing whole messages can panic
when a payload contains slices or maps. Hooks carry the outer `messaging.Msg`;
inspect its `Payload` for the protocol type.

Store complete messages in component state and port buffers. `Msg` preserves
concrete payload types through JSON, including nested messages; a separate
`messaging.Envelope` is unnecessary.

Switching networks now deliver the original message and concrete payload.
`packetization.AssembledMsg` and the `Delivery` role are removed; receivers use
their application protocol. `Flit.Msg` contains the full message only on flit 0.
Use `Flit.MsgID` and `Flit.Dst` for identification and routing on every flit,
rather than reading `Flit.Msg.ID` or `Flit.Msg.Dst`. The remaining transport
header contains `SeqID`, `NumFlitInMsg`, and `MsgTaskID`. Reassembly handles
out-of-order arrival and checkpoints without duplicating the payload per flit.
Existing checkpoints with the old flit/reassembly layout must be recreated.

### IDGenerator (V5)

Each simulation owns its counter. Components retain their simulation reference. There
is no process-global generator or sequential/parallel configuration switch.

```go
id := sim.NewID() // uint64, unique within this simulation
msg := messaging.Msg{ID: component.NewID(), Payload: ReadReq{}} // same counter
```

Event factories now take the allocated ID explicitly:
`timing.MakeEventBase(sim.NewID(), time, handlerID)` and
`ticking.MakeEvent(sim.NewID(), time, handlerID)`.

Separate simulations may reuse numeric IDs. Tracing associations are keyed by
the component itself and the message ID, so simulations that reuse IDs stay
apart.

Pass the simulation to builders with `WithSimulation(sim)`. All component and
package builders accept the shared `timing.Simulation` interface. A component
keeps its simulation to itself and allocates IDs with `NewID()`; engines have
no `NewID()`. For lightweight setups, create
`sim.MakeBuilder().Build()` once and share that instance with
all builders. A custom task owner implements `tasks.Owner`: `Name`,
`CurrentTime`, `NewID`, and `TaskTracker`, all backed by the same simulation.
Tracking does not require a hook interface. Monitors receive the simulation
through `Start(s)` so progress IDs come from the same counter.

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
pendingReqs := map[uint64]messaging.Msg{}

msg := messaging.Msg{
    ID: component.NewID(), // 1, 2, 3, ...
    Src: port.AsRemote(), Dst: lower,
    Payload: ReadReq{},
}
pendingReqs[msg.ID] = msg

// Later, matching response:
if original, ok := pendingReqs[rsp.RspTo]; ok {
    delete(pendingReqs, rsp.RspTo)
}

// Check if ID is empty:
if msg.ID == 0 { ... }
```

### Migration Checklist

- Change `map[string]...` keyed on IDs to `map[uint64]...`.
- Replace `== ""` / `!= ""` checks with `== 0` / `!= 0`.
- Replace `fmt.Sprintf`-based ID formatting with `strconv.FormatUint` or `%d`.
- Update tracing task ID comparisons from string to uint64.
- Replace global ID allocation with `sim.NewID()` or `component.NewID()`.
- Simulation checkpoints include the owned counter. Restore into fresh initialized instances.

---

## 3. Unified Control Protocol

**Motivation:** V4 had separate request/response types for each control
operation (flush, drain, restart). V5 consolidates them into a single
`memcontrolprotocol.Req` / `memcontrolprotocol.Rsp` payload pair with a
`Command` enum. Both travel inside `messaging.Msg`.

### V5 Types

```go
// package memcontrolprotocol
type Command int

const (
    CmdPause Command = iota // Freeze new and in-flight work
    CmdDrain               // Finish in-flight work, then remain paused
    CmdEnable              // Resume processing
    CmdReset               // Reset component state
    CmdInvalidate          // Drop filtered cache entries while paused/drained
    CmdFlush               // Write back dirty lines while paused/drained
)

type Req struct {
    Command   Command
    Addresses []uint64 // Invalidate/Flush filter; empty = all entries
    PID       vm.PID   // Filter; zero = all PIDs
}

type Rsp struct {
    Command Command
    Success bool
    Error   string
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

**After (V5) — one request type, `memcontrolprotocol.Req`, with a command:**
```go
// Draining a cache: in-flight work finishes, and the cache ends paused.
controlPort.Send(messaging.Msg{
    ID: comp.NewID(), Src: controlPort.AsRemote(), Dst: cacheControlPort,
    Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdDrain},
})

// After the drain acknowledgment, flush dirty lines to backing memory.
controlPort.Send(messaging.Msg{
    ID: comp.NewID(), Src: controlPort.AsRemote(), Dst: cacheControlPort,
    Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdFlush},
})

// After the flush acknowledgment, resume processing.
controlPort.Send(messaging.Msg{
    ID: comp.NewID(), Src: controlPort.AsRemote(), Dst: cacheControlPort,
    Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdEnable},
})

// One response payload type for every command; msg.RspTo identifies the request.
func handleControlRsp(msg messaging.Msg) {
    rsp, ok := msg.Payload.(memcontrolprotocol.Rsp)
    if !ok {
        panic("unexpected control payload")
    }
    if !rsp.Success {
        // rsp.Error names the reason, e.g. "unsupported"
        return
    }

    switch rsp.Command {
    case memcontrolprotocol.CmdDrain:
        // drain completed
    case memcontrolprotocol.CmdFlush:
        // flush completed
    case memcontrolprotocol.CmdEnable:
        // re-enabled
    }
}
```

### Migration Checklist

- Put `memcontrolprotocol.Req{Command: ...}` in `messaging.Msg.Payload`; put routing fields on the outer message.
- Replace `FlushReq` with `CmdFlush`, `DrainReq` with `CmdDrain`, and `RestartReq` with `CmdEnable` or `CmdReset`, as appropriate.
- Handle `msg.Payload.(memcontrolprotocol.Rsp)` and match `msg.RspTo` to the original message ID.
- Pause or drain before `CmdFlush`/`CmdInvalidate`. Wait for each command's acknowledgment before issuing a dependent command.
- Use `Addresses` and `PID` for filtered flush/invalidation; the earlier `DiscardInflight`, `InvalidateAfter`, and `PauseAfter` flags are not part of V5.

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
    ID        uint64         `json:"id"`
    At        VTimeInPicoSec `json:"time"`
    Handler   string         `json:"handler_id"`
    Secondary bool           `json:"secondary"`
}
```

### Handler Registration

Every `timing.Engine` has `RegisterHandler`:

```go
// v5/timing/engine.go
type Engine interface {
    // ...
    RegisterHandler(name string, handler Handler)
}
```

`Schedule` panics on an event whose handler is not registered, so a
misspelled or missing handler fails where the event is scheduled.

Components register themselves during construction. Every component model's
`Build` ends by registering the instance (`Register` in
`modeling/internal/base`): it registers the instance's ports,
registers the instance with the engine under its name (so events whose
`HandlerID()` is that name reach it), and registers it with the simulation:

```go
// v5/modeling/internal/base/base.go
func Register[S, T, R, P, M any](base *ComponentBase[S, T, R, P, M]) {
    registerPorts(base.simulation, &base.Ports)

    base.simulation.Engine().RegisterHandler(base.name, base.owner)

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

**Motivation:** An earlier V5 pre-release used a double-buffered model with
deep copies of `current` and `next`. That pre-release design was expensive and
error-prone; it was not the V4 component model. V5 now uses one mutable State
value per component. The cleanup below applies to code using that pre-release
design; V4 components should follow the component-model migration in section 6.

### V5 Dispatch

A V5 component hands each event it receives to its middlewares, which mutate
its one State in place. The dispatch logic in
`sim/modeling/ticking/component.go` includes:

```go
if base.Dispatch(c.pipeline, e) {
    c.ticks.TickLater()
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

The initial State comes from `State.Initialize` (or is the zero
value), and loading a checkpoint replaces it wholesale. Only the component's
own code writes it.

### Before / After

The pre-release double-buffered design required reading the old state,
writing a separate next state, and committing it after each tick. Remove that
copy/commit machinery when migrating pre-release components.

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
- For initialization, assign through the receiver in `(*State).Initialize(c *Comp)`.
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
| **Spec** | Configuration | System builder (defaults in `Definition.DefaultSpec`) | Scalars (bool, numbers, strings, and named types based on them), scalar slices/arrays, and flat records or slices of flat records with explicit JSON tags; record fields may be scalars or scalar slices. No maps, nested records, pointers, or interfaces. A ticking component's Spec has a `Freq timing.Freq` field. |
| **State** | Mutable runtime data, saved in checkpoints | Component (`State.Initialize`, or the zero value) | Pure data: scalars, slices, arrays, maps, nested structs. No pointers, ports, functions, channels. Use IDs for cross-references. Written only by the component's own code. |
| **Resources** | References to shared objects (storage, page table) | System builder | Not checkpointed; the rebuild supplies them again. `modeling.None` when there are none. |
| **Ports** | One `messaging.Port` field per port, `[]messaging.Port` per port group | System builder (`twowaybuffered.NewPort`) | Bound and registered by `BindPort`; topology is fixed at `Initialize`. A field may carry an `akita:"role=<protocol>.<role>"` tag. |
| **Middlewares** | The behavior: one exported pointer field per middleware | Component (`Middlewares.Initialize`) | Each implements `Handle(e timing.Event) bool`; they run in field order and hold only references. |

Hooks are not a sixth struct: every component embeds `hooking.HookableBase`,
so it accepts hooks and fires its own hook points (`InvokeHook`) without
affecting simulation logic.

### Three Component Models

Each model is a sub-package of `modeling` with the same shape — a `Definition`,
a `Builder`, and a generic `Component` over the five structs. They differ only
in which events reach the middlewares:

| Model | Events that reach the middlewares | Use it for |
|-------|-----------------------------------|------------|
| `modeling/ticking` | a `ticking.Event` every cycle while any middleware makes progress; port activity restarts ticking (`TickLater` starts it) | work that advances cycle by cycle: pipelines, caches, switches. The default, and the usual target for a V4 ticking component. |
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

    // Bottom is the remote port of the unit that the reorder buffer
    // forwards requests to. The reorder buffer rewrites the Dst of every
    // shadow request to this value.
    Bottom messaging.RemotePort `json:"bottom"`
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
}

func (m *Middlewares) Initialize(c *Comp) {
    *m = Middlewares{Pipeline: &middleware{comp: c}}
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

`DefaultSpec` must be a keyed literal with constant leaves, because the `inspect` package reads the
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
spec.Bottom = bottomUnit.AsRemote()

r := rob.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    Build("ROB")
r.BindPort("Top", twowaybuffered.NewPort(4, 4))
r.BindPort("Bottom", twowaybuffered.NewPort(4, 4))
r.BindPort("Control", twowaybuffered.NewPort(4, 4))

conn.BindPort(r.Ports.Top)
```

`Build` validates the Spec and State shapes and registers the component.
`BindPort` assigns each declared port; `Initialize` creates State and Middlewares
after wiring. Access ports through fields such as `r.Ports.Top`.

### Defining Components in V5: Philosophy and Patterns

V5 unifies how components are modeled and wired. Each component type is five structs — Spec, State, Resources, Ports, and Middlewares — and a `Definition`, declared in a package of its own; a component model turns them into a running component. The goals are: declarative configuration, local and serializable runtime state, explicit wiring, testability, and deterministic snapshot/restore.

#### Core Principles

1. Spec (immutable configuration)
   - Describes behavior and dependencies using only scalars (bool, numbers, strings, and named types based on them, such as `timing.Freq` or an enum-like `type Mode string`), scalar slices/arrays, and flat records or slices of flat records with explicit JSON tags; record fields may be scalars or scalar slices. No maps or nested records.
   - Routing dependencies are plain values such as `Bottom mem.AddressMapper`; its destinations and parameters fully specify the behavior.
   - No pointers or live objects in Spec. Keep it JSON/YAML‑friendly and hashable.
   - Defaults live in `Definition.DefaultSpec`; the system builder copies it, changes fields, and passes the result to `WithSpec`.

2. State (mutable runtime data)
   - Pure data only: scalars, slices, arrays, maps with string or integer keys, and simple nested structs.
   - No live handles, functions, channels, or ports in State.
   - All cross‑references use stable identifiers (IDs), never in‑memory pointers.
   - Written only by the component's own code: its `State.Initialize`, its middlewares, and functions its package exports for the system builder (such as `ping.SchedulePing`).
   - A checkpoint serializes the State; loading one replaces it wholesale.

3. Resources (references supplied by the system builder)
   - References to shared objects — a `*mem.Storage` or a `vm.PageTable` — passed with `WithResources`.
   - Not checkpointed. A shared object that holds data checkpoints itself as a registered resource, and the rebuild supplies the same references again.

4. Ports (declared by the component, created by the system builder)
   - A component declares the ports it has as the fields of its `Ports` struct, tagged with the protocol roles they speak, but never constructs the instances or owns connections.
   - The system builder creates each port with `twowaybuffered.NewPort(in, out)`, choosing its buffer sizes, and binds each declared slot with `component.BindPort("Field", port)`.
   - Middlewares reach ports as fields (`m.comp.Ports.Top`), checked by the compiler.

5. Middlewares (ordered, holding only references)
   - Each middleware implements `Handle(e timing.Event) bool` and is an exported pointer field of `Middlewares`; the component passes every event to them in field order. Created in `Middlewares.Initialize`.
   - A middleware holds the component pointer and nothing mutable; everything that changes lives in State.
   - In a ticking component, prefer tick‑driven countdowns/backpressure over ad‑hoc scheduled events for simpler snapshots and determinism. When the behavior is naturally a set of timed events, use the event model instead.

#### Dependency Injection and Shared State

- Strategy injection (e.g., address conversion)
  - Keep configuration in Spec as scalars or flat records, not as a live object.
  - Route directly through `mem.AddressMapper` values in Spec; other strategies may derive private lookup tables at initialization. Builtin components do not accept mapper overrides through Resources.
  - On restore, reconstruct from Spec; never serialize strategy objects.

- Emulation state (e.g., memory storage)
  - Treat as shared state separate from timing logic. The system builder creates the `*mem.Storage` and passes it through Resources (`dram.Resources{Storage: storage}`); several components may share one.
  - The storage checkpoints itself once, as a registered resource; components checkpoint only their own State.

#### Build and Wire

1. Configure Spec structs and build components with their final full names.
2. Create unnamed ports and bind them with `component.BindPort("Field", port)`.
3. Build connections and attach ports with `connection.BindPort(port)`.
4. Call `simulation.Initialize()` before accessing runtime State or Middlewares,
   seeding work, or running. The initialized topology is fixed.

#### Determinism and Introspection

- Determinism: avoid non‑deterministic IDs or iteration order; snapshot ID generators; canonicalize map iteration by sorting.
- Introspection: `comp.Spec` holds the effective Spec and `comp.State` can be dumped for debugging; the `inspect` package reads each `Definition`, its ports, and their roles without running the code.
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

### Lists in Spec

V5 Spec accepts scalars, scalar arrays/slices, flat records such as
`mem.AddressMapper`, and slices of flat records such as `[]Link`. Each record
field must be an exported scalar or scalar slice with an explicit, unique JSON
name. Nested records, other nested containers, maps, pointers, interfaces,
and record arrays are rejected; runtime validation and `inspect` use the same
rules. The inspector exports record fields and literal defaults in its schema.

| The list is… | Put it in | Example |
|---|---|---|
| Destinations or address mapping | Spec | `Bottom mem.AddressMapper`: destinations, interleaving size, range, fallback |
| Per-link configuration | Spec | `Links []Link`, index-aligned with `Ports.Links` |
| Runtime data | State | Queues and in-flight transactions |
| Shared live objects | Resources | Storage and page tables |

Build copies every Spec slice, including scalar slices inside records. Do not mutate the instance's Spec afterward; it is
included in checkpoint compatibility checks. Fill remote addresses with planned
full names, or bind destination ports before calling `AsRemote()`.

Memory middlewares route directly through the mapper value in Spec. Other
components may derive lookup tables at initialization. Resources must not provide
a second routing override.
Resources are rebuilt separately; shared registered objects checkpoint themselves.

Move component-specific checks into `(*State).Initialize(c *Comp)` or
`(*Middlewares).Initialize(c *Comp)`, before using the checked values. These methods
run after generic port validation and topology freezing. A component-specific
failure now panics during Initialize and requires rebuilding the simulation;
it is no longer a Build-time check or a recoverable validation error.

Remove Definition.NewState, NewMiddlewares, ValidateSpec, and ValidateSetup.
Definition contains only DefaultSpec. Convert constructors that returned values
into Initialize methods that assign through their pointer receiver. Either method
may be omitted when the element's zero value works; nil middleware pointers still
fail initialization. Do not mutate Spec, Resources, Ports, or another component's
State from an initializer. Checkpoint data and middleware Handle signatures are
unchanged by this migration.

### Migration Checklist

- Give each component type its own package, and choose its model: `modeling/ticking` for cycle-by-cycle work, `modeling/wakeup` or `modeling/event` for clockless components.
- Split each component's configuration into Spec (scalar user settings, including `Freq timing.Freq` for a ticking component), State (data that changes while simulating), and Resources (references to external objects and the wiring derived from them).
- Classify lists and nested values using the table above; flatten unsupported containers.
- Express strategy choices as scalars or flat configuration records. Records may contain scalar slices but cannot nest.
- Give every Spec and State field a `json` tag, and make sure no two fields share a JSON name.
- Replace port fields, port declarations, and port lookups by name with a `Ports` struct: one `messaging.Port` field per port, `[]messaging.Port` per port group, each tagged `akita:"role=<protocol>.<role>"` when it speaks a protocol role, where `<protocol>` is the import path of the package that defines the protocol. Reach ports as `comp.Ports.X`.
- Turn each per-tick method into `Handle(e timing.Event) bool`, list the middlewares as exported pointer fields of a `Middlewares` struct in the order they run, and create them in a `(*Middlewares).Initialize(c *Comp)` method. Move any mutable middleware field into State.
- Declare `type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]` and a `Definition` containing only `DefaultSpec`. Initialize State and Middlewares through their own optional methods.
- Turn exported methods on the component into package functions that take `*Comp`.
- In the system builder, create every port with `twowaybuffered.NewPort(in, out)` and build with `Definition.Builder().WithSimulation(sim).WithSpec(spec).WithResources(res).Build(name)`. Bind every declared port and connection, then call `sim.Initialize()`. After initialization, start a component that begins work on its own with `TickLater()`.
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
    Build("DRAM")
ctrl.BindPort("Top", twowaybuffered.NewPort(1024, 1024))
ctrl.BindPort("Control", twowaybuffered.NewPort(4, 4))
```

### Statistics

The DRAM controller tracks runtime statistics, read through functions:

```go
// Latency
avgRead := dram.AverageReadLatency(ctrl)   // cycles
avgWrite := dram.AverageWriteLatency(ctrl) // cycles

// Bandwidth
readBW := dram.ReadBandwidth(ctrl)   // bytes per cycle
writeBW := dram.WriteBandwidth(ctrl) // bytes per cycle

// Row buffer
hitRate := dram.RowBufferHitRate(ctrl) // 0.0 to 1.0

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
    Build("DRAM")
ctrl.BindPort("Top", twowaybuffered.NewPort(4, 4))
ctrl.BindPort("Control", twowaybuffered.NewPort(4, 4))
```

---

## 8. Port Creation API

In V4, ports were created internally by component builders. In V5, the
component owns its port *topology* — the fields of its `Ports` struct say
which ports it has — but it does not create the instances. The system builder
creates each port with `twowaybuffered.NewPort` and assigns it with
`component.BindPort` after Build. This makes wiring explicit and lets ports be sized or
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
    Build("Cache")
cache.BindPort("Top", twowaybuffered.NewPort(4, 4))
cache.BindPort("Bottom", twowaybuffered.NewPort(4, 4))
cache.BindPort("Control", twowaybuffered.NewPort(4, 4))

conn.BindPort(cache.Ports.Top)
```

`component.BindPort("Top", port)` assigns the name, owner, field, and simulation
registration together. It rejects an unknown slot, an occupied slot, or a port
that already has an owner before changing the topology. `Initialize` reports
missing ports and holes in port groups.

### Custom port owners

Owners outside the component models use the lower-level
`port.BindOwner(owner, fullName)` and register the port themselves. Connections
use `port.BindConnection(connection)` to set the reverse attachment; normal
assembly code calls `connection.BindPort(port)` so both sides are updated.
An unowned port has no remote address. Use a planned full address during
configuration, or call `AsRemote()` after binding.

---

## 9. Queueing

The `queueing` package provides generic buffer and pipeline value types. V4's interface/implementation pattern (`sim.Buffer`, `pipelining.Pipeline`) is replaced by `queueing.Buffer[T]` and `queueing.Pipeline[T]`, created with `MakeBuffer` and `MakePipeline`. Both are values, so they embed directly in a component's State, and they serialize to JSON for checkpoints.

### Key Changes from V4 to V5

| V4 | V5 |
|---|---|
| `sim.NewBuffer("MyBuffer", 100)`, returning the `sim.Buffer` interface | `queueing.MakeBuffer[int](100)`, returning a `queueing.Buffer[int]` value |
| A `pipelining` builder with `WithNumStage`, `WithCyclePerStage`, and `WithPostPipelineBuffer` | `queueing.MakePipeline[int](width, numStages)`; `Tick(sink)` moves completed items into a sink, such as a buffer |
| A buffer has a name and hook positions | A buffer has neither; a component's State field already names it |

```go
// V4
buffer := sim.NewBuffer("MyBuffer", 100)

// V5
buffer := queueing.MakeBuffer[int](100)
pipeline := queueing.MakePipeline[int](1, 5) // 1 lane, 5 stages
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

## 10. CLI Limitations

The current CLI has `akita component --create` and `akita check`, but both
still target the legacy builder-based component layout. Their generated code
and validation rules do not support the V5 component models. There are no
`akita component-lint` or `akita component-create` commands.

For V5, follow the [component tutorial](components/01_what_is_a_component.md),
use a package-level `Definition`, and add the appropriate
`modelingtest.CheckTicking`, `CheckWakeup`, or `CheckEvent` test. The `inspect`
package reads component definitions without constructing a simulation.

---

## 11. CI Validation

`.github/workflows/akita_test.yml` runs on pushes to `main` and on pull requests.
It builds and tests the Go packages, generates mocks, runs lint and memory/NoC
acceptance tests, validates DRAM, and builds/tests the Daisen and monitoring
frontends. It also installs the locked `doc-site` dependencies and builds the
site with broken-link and anchor checks enabled. Commands run from the
repository root or their explicit working directory; there is no `v5/`
subdirectory. Runner selection is recorded in the workflow.
