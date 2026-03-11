# How to Create a Component in Akita V5

This guide explains how to create a simulation component using the Akita V5
framework. It covers every concept you need — from the anatomy of a component
to complete worked examples.

> **Reference code locations:**
>
> | Package | Path | Description |
> |---------|------|-------------|
> | `modeling` | `v5/modeling/` | Generic `Component[S, T]`, `Builder`, validation, save/load |
> | `tickingping` | `v5/examples/tickingping/` | Simplest complete component |
> | `idealmemcontroller` | `v5/mem/idealmemcontroller/` | Intermediate component with two middlewares |
> | Migration guide | `v5/migration.md` | V4 → V5 changes |

---

## 1. Component Anatomy

Every V5 component is built from five orthogonal parts:

```
┌────────────────────────────────────────────┐
│              Component[S, T]               │
│                                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │   Spec   │  │  State   │  │  Ports   │ │
│  │(immutable│  │(mutable  │  │(external │ │
│  │  config) │  │ runtime) │  │  I/O)    │ │
│  └──────────┘  └──────────┘  └──────────┘ │
│                                            │
│  ┌──────────────────────────────────────┐  │
│  │          Middleware Pipeline         │  │
│  │  mw1.Tick() → mw2.Tick() → ...      │  │
│  └──────────────────────────────────────┘  │
│                                            │
│  ┌──────────────────────────────────────┐  │
│  │             Hooks                    │  │
│  │  (tracing, metrics, debugging)       │  │
│  └──────────────────────────────────────┘  │
└────────────────────────────────────────────┘
```

### 1.1 Spec (Immutable Configuration)

The **Spec** is a plain struct that captures everything about *how* the
component is configured. Once set at build time, it never changes during
simulation.

- Holds parameters like latency, width, frequency, and strategy descriptors.
- Must contain **only primitives** (bool, int/uint variants, float32/float64, string), slices of primitives, and maps with string keys and primitive values.
- No pointers, interfaces, functions, channels, or nested structs allowed.
- Must be JSON-serializable.

### 1.2 State (Mutable Runtime Data)

The **State** is a plain struct that captures everything about the component's
*current runtime condition*. It changes every tick.

- Holds in-flight transactions, counters, queues, mode flags, etc.
- Must contain only primitives, **nested structs**, slices/maps of primitives or structs.
- No pointers, interfaces, functions, or channels.
- Cross-references between components use **string IDs**, never pointers.
- Must be JSON-serializable (for checkpoint/restore).

### 1.3 Ports

Ports are the component's communication endpoints. In V5, ports are created
**externally** and injected into the component via the builder. This makes
wiring explicit and decouples port creation from component construction.

### 1.4 Middleware

Middlewares implement the per-tick behavior pipeline. Each middleware is a
struct that implements the `sim.Middleware` interface (a single `Tick() bool`
method). The `MiddlewareHolder` calls each middleware's `Tick()` in order
and returns `true` if any middleware made progress.

### 1.5 Hooks

Hooks provide non-intrusive observation points for tracing, metrics, and
debugging. Components inherit hook support from `sim.ComponentBase` via
`sim.HookableBase`. Hooks can be attached to components, ports, and engines
at various positions (e.g., before/after events, message send/receive).

---

## 2. `modeling.Component[S, T]` and `modeling.NewBuilder`

### 2.1 The Generic Component

`modeling.Component[S, T]` is the core type for all V5 components. It is
parameterized by two type arguments:

- **`S`** — the Spec type (immutable configuration)
- **`T`** — the State type (mutable runtime data)

```go
// From v5/modeling/component.go
type Component[S any, T any] struct {
    *sim.TickingComponent
    sim.MiddlewareHolder

    spec  S
    state T
}
```

It embeds:
- **`sim.TickingComponent`** — provides tick-based lifecycle management
  (`TickLater()`, `TickNow()`, `CurrentTime()`, event handling,
  `NotifyRecv`/`NotifyPortFree` callbacks, port management via
  `ComponentBase`)
- **`sim.MiddlewareHolder`** — manages the ordered middleware pipeline

Key methods:

| Method | Description |
|--------|-------------|
| `GetSpec() S` | Returns the immutable specification |
| `GetState() T` | Returns a copy of the current state |
| `SetState(state T)` | Replaces the component state |
| `Tick() bool` | Delegates to the middleware pipeline |
| `AddMiddleware(mw)` | Appends a middleware to the pipeline |
| `AddPort(name, port)` | Registers a port with the component |
| `Name() string` | Returns the component's name |
| `CurrentTime()` | Returns the current simulation time |
| `TickLater()` | Schedules a tick on the next cycle |
| `SaveState(w)` | Serializes spec + state to JSON |
| `LoadState(r)` | Deserializes spec + state from JSON |
| `ResetTick()` | Resets the tick scheduler (for checkpoint restore) |
| `ResetAndRestartTick()` | Resets tick scheduler and schedules a new tick |

### 2.2 The Builder

`modeling.NewBuilder[S, T]()` creates a builder for `Component[S, T]`:

```go
// From v5/modeling/builder.go
type Builder[S any, T any] struct {
    engine sim.Engine
    freq   sim.Freq
    spec   S
}
```

Builder methods (all return a new `Builder` — immutable value-receiver pattern):

| Method | Description |
|--------|-------------|
| `WithEngine(engine)` | Sets the simulation engine |
| `WithFreq(freq)` | Sets the component's clock frequency |
| `WithSpec(spec)` | Sets the immutable Spec |
| `Build(name) *Component[S,T]` | Constructs the component |

The `Build` method creates the underlying `sim.TickingComponent` and wires
it into the simulation engine.

---

## 3. Spec Design Rules

### Rules

1. The Spec must be a **plain Go struct**.
2. Fields must be **primitive types only**: `bool`, `int`, `int8`–`int64`,
   `uint`, `uint8`–`uint64`, `float32`, `float64`, `string`.
3. **Slices of primitives** are allowed (e.g., `[]int`, `[]string`).
4. **Maps with string keys and primitive values** are allowed
   (e.g., `map[string]int`).
5. **No nested structs, pointers, interfaces, functions, or channels.**
6. All fields should have `json:"..."` tags for serialization.
7. Fields tagged `json:"-"` are skipped during validation.

### Validation

Use `modeling.ValidateSpec(v)` to verify at runtime:

```go
spec := MySpec{Width: 4, Latency: 100}
if err := modeling.ValidateSpec(spec); err != nil {
    panic(fmt.Sprintf("invalid spec: %v", err))
}
```

### What passes / what fails

```go
// ✅ Valid Spec — primitives only
type GoodSpec struct {
    Width   int               `json:"width"`
    Latency int               `json:"latency"`
    Name    string            `json:"name"`
    Labels  map[string]string `json:"labels"`
    Ids     []int             `json:"ids"`
}

// ❌ Invalid — nested struct
type BadSpec1 struct {
    Inner struct{ X int }
}

// ❌ Invalid — pointer field
type BadSpec2 struct {
    P *int
}

// ❌ Invalid — interface field
type BadSpec3 struct {
    I interface{}
}
```

### Design Tips

- Express strategy dependencies (e.g., address conversion) as primitive
  descriptors in Spec (e.g., `AddrConvKind string`), then resolve them to
  concrete implementations in the builder.
- Express shared resource references as string IDs
  (e.g., `StorageRef string`), not as pointers.

---

## 4. State Design Rules

### Rules

1. The State must be a **plain Go struct**.
2. All Spec-allowed types are also allowed in State.
3. Additionally, **nested structs** are allowed (and slices/maps of structs).
4. Nested structs must themselves follow the same rules recursively — no
   pointers, interfaces, functions, or channels at any nesting depth.
5. Cross-references to other components must use **string IDs**, not pointers.
6. All fields should have `json:"..."` tags for serialization.

### Validation

Use `modeling.ValidateState(v)` to verify at runtime:

```go
state := MyState{Counter: 0}
if err := modeling.ValidateState(state); err != nil {
    panic(fmt.Sprintf("invalid state: %v", err))
}
```

### What passes / what fails

```go
// ✅ Valid State — primitives + nested structs
type Transaction struct {
    CycleLeft int    `json:"cycle_left"`
    ReqID     string `json:"req_id"`
    IsRead    bool   `json:"is_read"`
}

type GoodState struct {
    Counter      int             `json:"counter"`
    Transactions []Transaction   `json:"transactions"`
    Lookup       map[string]Transaction `json:"lookup"`
}

// ❌ Invalid — pointer field
type BadState1 struct {
    P *int
}

// ❌ Invalid — function field
type BadState2 struct {
    F func()
}
```

### Spec vs. State Comparison

| Feature | Spec | State |
|---------|------|-------|
| Primitives | ✅ | ✅ |
| Slices of primitives | ✅ | ✅ |
| Maps (string keys, primitive values) | ✅ | ✅ |
| Nested structs | ❌ | ✅ |
| Slices/maps of structs | ❌ | ✅ |
| Pointers | ❌ | ❌ |
| Interfaces | ❌ | ❌ |
| Functions | ❌ | ❌ |
| Channels | ❌ | ❌ |

---

## 5. Middleware Implementation

### 5.1 The Middleware Interface

A middleware implements `sim.Middleware`:

```go
// From v5/sim/middleware.go
type Middleware interface {
    Tick() bool
}
```

`Tick()` returns `true` if the middleware made progress (processed a message,
decremented a counter, sent a response, etc.). The `MiddlewareHolder` calls
each registered middleware in order and returns `true` if **any** middleware
made progress. When progress is made, the component automatically schedules
another tick on the next cycle.

### 5.2 Middleware Structure

A middleware is typically a struct that embeds a pointer to the outer component
(`*Comp`). This gives it access to Spec, State, and Ports:

```go
type memMiddleware struct {
    *Comp
}
```

### 5.3 Accessing Spec, State, and Ports

Inside a middleware's `Tick()` method:

```go
func (m *memMiddleware) Tick() bool {
    // Read immutable configuration
    spec := m.Component.GetSpec()
    width := spec.Width

    // Read/modify mutable state
    state := m.Component.GetState()
    state.Counter++
    m.Component.SetState(state)

    // Access ports (stored as fields on Comp)
    msg := m.topPort.PeekIncoming()
    // ...

    return true
}
```

**Important pattern:** `GetState()` returns a **copy** of the state struct.
After modifying the copy, you must call `SetState(state)` to write it back.

### 5.4 Tick() Logic Pattern

A typical `Tick()` method orchestrates multiple sub-operations:

```go
func (m *middleware) Tick() bool {
    madeProgress := false

    madeProgress = m.sendResponses()  || madeProgress
    madeProgress = m.processTimers()  || madeProgress
    madeProgress = m.acceptRequests() || madeProgress

    return madeProgress
}
```

Each sub-operation:
1. Checks if there's work to do (peek at ports, check state)
2. Does the work (send messages, modify state)
3. Returns `true` if it did something, `false` otherwise

### 5.5 Multiple Middlewares

A component can have multiple middlewares, each responsible for a different
concern. The middlewares are called in the order they were added. For example,
the ideal memory controller has:

1. **ctrlMiddleware** — handles enable/pause/drain control commands
2. **memMiddleware** — handles memory read/write requests

```go
ctrlMiddleware := &ctrlMiddleware{Comp: c}
c.AddMiddleware(ctrlMiddleware)
funcMiddleware := &memMiddleware{Comp: c}
c.AddMiddleware(funcMiddleware)
```

---

## 6. Port Setup

### 6.1 V5 Port Philosophy

In V5, ports are created **externally** and injected into components via
builder methods. This is a deliberate departure from V4 where components
created their own ports internally. The V5 approach:

- Makes topology wiring explicit
- Allows ports to be configured before a component is built
- Decouples port creation from component construction

### 6.2 Creating Ports

Use `sim.NewPort` to create a port:

```go
port := sim.NewPort(nil, inBufSize, outBufSize, "ComponentName.PortName")
```

Parameters:
- First argument: component (can be `nil` initially — set via `SetComponent` later)
- `inBufSize`: incoming message buffer capacity
- `outBufSize`: outgoing message buffer capacity
- Name: conventionally `"ComponentName.PortName"`

### 6.3 Builder WithXxxPort Methods

Component builders expose `WithXxxPort()` methods for each port:

```go
type Builder struct {
    engine  sim.Engine
    freq    sim.Freq
    topPort sim.Port
    ctrlPort sim.Port
}

func (b Builder) WithTopPort(port sim.Port) Builder {
    b.topPort = port
    return b
}

func (b Builder) WithCtrlPort(port sim.Port) Builder {
    b.ctrlPort = port
    return b
}
```

### 6.4 SetComponent

After building a component, each port must be associated with it via
`SetComponent`. This is done inside the builder's `Build` method:

```go
func (b Builder) Build(name string) *Comp {
    // ... create the component ...

    c.topPort = b.topPort
    c.topPort.SetComponent(c)      // Associate port with component
    c.AddPort("Top", c.topPort)    // Register port by name

    c.ctrlPort = b.ctrlPort
    c.ctrlPort.SetComponent(c)
    c.AddPort("Control", c.ctrlPort)

    return c
}
```

`SetComponent` is critical because:
- It lets the port call `NotifyRecv` and `NotifyPortFree` on the owning
  component, which triggers the tick-based lifecycle
- Without it, the component won't wake up when messages arrive

### 6.5 Connecting Ports

After building components, connect their ports using a connection:

```go
conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Conn")

conn.PlugIn(agentA.OutPort)
conn.PlugIn(agentB.OutPort)
```

### 6.6 Full Wiring Example

```go
// 1. Create ports externally
topPort := sim.NewPort(nil, 16, 16, "MemCtrl.TopPort")
ctrlPort := sim.NewPort(nil, 4, 4, "MemCtrl.CtrlPort")

// 2. Build component, injecting ports
memCtrl := idealmemcontroller.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithTopPort(topPort).
    WithCtrlPort(ctrlPort).
    Build("MemCtrl")

// 3. Connect ports to other components
conn.PlugIn(topPort)
conn.PlugIn(otherComponent.SomePort)
```

---

## 7. Save/Load via SaveState/LoadState

### 7.1 How It Works

`Component[S, T]` provides built-in checkpoint support through JSON
serialization of both Spec and State:

```go
// From v5/modeling/saveload.go

// SaveState marshals spec + state as JSON to a writer.
func (c *Component[S, T]) SaveState(w io.Writer) error

// LoadState reads JSON from a reader and restores spec + state.
func (c *Component[S, T]) LoadState(r io.Reader) error
```

The serialized format is:

```json
{
  "spec": { ... },
  "state": { ... }
}
```

### 7.2 Serialization Constraints

Both Spec and State must be JSON-serializable. This is enforced by the
validation rules:

- No pointers, interfaces, functions, or channels
- Use `json:"..."` tags on all fields
- Use `json:"-"` to exclude fields from serialization (and validation)
- Use `json:",omitempty"` for optional fields

### 7.3 Save Example

```go
var buf bytes.Buffer
if err := comp.SaveState(&buf); err != nil {
    log.Fatalf("save failed: %v", err)
}
// buf.Bytes() now contains the JSON snapshot
```

### 7.4 Load Example

```go
// Create a fresh component (Spec will be overwritten by LoadState)
comp2 := modeling.NewBuilder[MySpec, MyState]().
    WithEngine(engine).
    WithFreq(freq).
    Build("RestoredComp")

if err := comp2.LoadState(&buf); err != nil {
    log.Fatalf("load failed: %v", err)
}

// After loading, reset the tick scheduler
comp2.ResetAndRestartTick()
```

### 7.5 After Loading: Reset Ticks

After restoring state from a checkpoint, the tick scheduler must be reset
so that future `TickLater()` calls can schedule new events:

```go
// Reset only — don't start ticking until triggered
comp.ResetTick()

// Or reset and immediately schedule a tick
comp.ResetAndRestartTick()
```

---

## 8. Builder Pattern

### 8.1 Two-Level Builder Pattern

V5 components use a two-level builder pattern:

1. **Inner builder**: `modeling.NewBuilder[S, T]()` — constructs the generic
   `modeling.Component[S, T]`
2. **Outer builder**: Component-specific builder — constructs the full
   component with ports, middlewares, storage, etc.

### 8.2 Outer Builder Template

```go
package mycomponent

import (
    "github.com/sarchlab/akita/v5/modeling"
    "github.com/sarchlab/akita/v5/sim"
)

type Builder struct {
    engine  sim.Engine
    freq    sim.Freq
    spec    *Spec           // component-specific defaults
    port    sim.Port        // externally created port
}

// MakeBuilder returns a new Builder with sensible defaults.
func MakeBuilder() Builder {
    return Builder{
        freq: 1 * sim.GHz,
        spec: &Spec{
            Latency: 100,
            Width:   1,
        },
    }
}

// WithEngine sets the simulation engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
    b.engine = engine
    return b
}

// WithFreq sets the clock frequency.
func (b Builder) WithFreq(freq sim.Freq) Builder {
    b.freq = freq
    return b
}

// WithSpec overrides the full spec.
func (b Builder) WithSpec(spec Spec) Builder {
    b.spec = &spec
    return b
}

// WithPort injects the port.
func (b Builder) WithPort(port sim.Port) Builder {
    b.port = port
    return b
}

// Build creates the component.
func (b Builder) Build(name string) *Comp {
    // 1. Use the inner builder to create the modeling.Component
    modelComp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(*b.spec).
        Build(name)

    // 2. Set initial state
    modelComp.SetState(State{})

    // 3. Create the outer component
    c := &Comp{
        Component: modelComp,
    }

    // 4. Add middlewares
    mw := &middleware{Comp: c}
    c.AddMiddleware(mw)

    // 5. Wire ports
    c.port = b.port
    c.port.SetComponent(c)
    c.AddPort("Main", c.port)

    return c
}
```

### 8.3 Key Points

- **Value receivers on builder methods** — each `With...` method returns a
  new `Builder` value, enabling method chaining without mutation.
- **Ports are always external** — the builder accepts ports via `WithXxxPort`
  and wires them in `Build`.
- **`SetComponent` is always called in `Build`** — this connects the port's
  lifecycle to the component.
- **Defaults in `MakeBuilder`** — provide sensible defaults so callers only
  override what they need.

---

## 9. Complete Walkthroughs

### 9.1 tickingping — Simple Component

> Source: `v5/examples/tickingping/`

The `tickingping` component sends ping requests and responds to them. Two
instances are connected and exchange pings, measuring round-trip time.

#### 9.1.1 Messages

```go
// comp.go
type PingReq struct {
    sim.MsgMeta
    SeqID int
}

type PingRsp struct {
    sim.MsgMeta
    SeqID int
}
```

Messages embed `sim.MsgMeta` which provides `ID`, `Src`, `Dst`, and `RspTo`
fields required by the messaging system.

#### 9.1.2 Spec

```go
// comp.go
type Spec struct{}
```

This component has no configurable parameters, so the Spec is an empty struct.
It still satisfies the Spec constraints (empty struct = valid).

#### 9.1.3 State

```go
// comp.go
type pingTransactionState struct {
    SeqID     int            `json:"seq_id"`
    CycleLeft int            `json:"cycle_left"`
    ReqID     string         `json:"req_id"`
    ReqSrc    sim.RemotePort `json:"req_src"`
}

type State struct {
    StartTimes          []float64              `json:"start_times"`
    NextSeqID           int                    `json:"next_seq_id"`
    NumPingNeedToSend   int                    `json:"num_ping_need_to_send"`
    PingDst             sim.RemotePort         `json:"ping_dst"`
    CurrentTransactions []pingTransactionState `json:"current_transactions"`
}
```

Key design choices:
- `PingDst` is a `sim.RemotePort` (which is `string`) — not a pointer to a
  port object. This makes it serializable.
- `CurrentTransactions` uses a nested struct `pingTransactionState` — allowed
  in State but not in Spec.
- `ReqSrc` is also a `RemotePort` string, not a pointer.

#### 9.1.4 Comp (Outer Component)

```go
// comp.go
type Comp struct {
    *modeling.Component[Spec, State]

    OutPort sim.Port
}
```

The outer `Comp` struct embeds `*modeling.Component[Spec, State]` and adds
the port as a concrete field. Ports are stored on the outer component (not
in State) because they are live objects that cannot be serialized.

#### 9.1.5 Middleware

```go
// comp.go
type middleware struct {
    *Comp
}

func (m *middleware) Tick() bool {
    madeProgress := false

    madeProgress = m.sendRsp() || madeProgress
    madeProgress = m.sendPing() || madeProgress
    madeProgress = m.countDown() || madeProgress
    madeProgress = m.processInput() || madeProgress

    return madeProgress
}
```

The middleware embeds `*Comp`, giving it access to `m.Component.GetSpec()`,
`m.Component.GetState()`, `m.Component.SetState(...)`, and `m.OutPort`.

Each sub-method follows the pattern:
1. Read state via `m.Component.GetState()`
2. Check if there's work to do
3. Do the work (modify state, send messages)
4. Write state back via `m.Component.SetState(state)`
5. Return whether progress was made

Example — processing incoming messages:

```go
func (m *middleware) processInput() bool {
    msgI := m.OutPort.PeekIncoming()
    if msgI == nil {
        return false
    }

    switch msg := msgI.(type) {
    case *PingReq:
        state := m.Component.GetState()
        trans := pingTransactionState{
            SeqID:     msg.SeqID,
            CycleLeft: 2,
            ReqID:     msg.ID,
            ReqSrc:    msg.Src,
        }
        state.CurrentTransactions = append(state.CurrentTransactions, trans)
        m.Component.SetState(state)
        m.OutPort.RetrieveIncoming()
    case *PingRsp:
        // handle response...
        m.OutPort.RetrieveIncoming()
    }

    return true
}
```

Note the peek-then-retrieve pattern: `PeekIncoming()` checks without
consuming; `RetrieveIncoming()` removes the message from the buffer.

#### 9.1.6 Builder

```go
// builder.go
type Builder struct {
    engine  sim.Engine
    freq    sim.Freq
    outPort sim.Port
}

func MakeBuilder() Builder {
    return Builder{}
}

func (b Builder) WithEngine(engine sim.Engine) Builder {
    b.engine = engine
    return b
}

func (b Builder) WithFreq(freq sim.Freq) Builder {
    b.freq = freq
    return b
}

func (b Builder) WithOutPort(port sim.Port) Builder {
    b.outPort = port
    return b
}

func (b Builder) Build(name string) *Comp {
    // Inner builder creates the modeling.Component
    modelComp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(Spec{}).
        Build(name)
    modelComp.SetState(State{})

    // Outer component
    c := &Comp{
        Component: modelComp,
    }

    // Add middleware
    mw := &middleware{Comp: c}
    c.AddMiddleware(mw)

    // Wire port
    c.OutPort = b.outPort
    c.OutPort.SetComponent(c)
    c.AddPort("Out", c.OutPort)

    return c
}
```

#### 9.1.7 Usage

```go
// example_test.go
func Example() {
    engine := sim.NewSerialEngine()

    agentA := MakeBuilder().
        WithEngine(engine).
        WithFreq(1 * sim.Hz).
        WithOutPort(sim.NewPort(nil, 4, 4, "AgentA.OutPort")).
        Build("AgentA")

    agentB := MakeBuilder().
        WithEngine(engine).
        WithFreq(1 * sim.Hz).
        WithOutPort(sim.NewPort(nil, 4, 4, "AgentB.OutPort")).
        Build("AgentB")

    conn := directconnection.MakeBuilder().
        WithEngine(engine).
        WithFreq(1 * sim.GHz).
        Build("Conn")
    conn.PlugIn(agentA.OutPort)
    conn.PlugIn(agentB.OutPort)

    // Initialize state
    state := agentA.GetState()
    state.PingDst = agentB.OutPort.AsRemote()
    state.NumPingNeedToSend = 2
    agentA.SetState(state)

    // Kick off simulation
    agentA.TickLater()
    engine.Run()

    // Output:
    // Ping 0, 5.00
    // Ping 1, 5.00
}
```

---

### 9.2 idealmemcontroller — Intermediate Component

> Source: `v5/mem/idealmemcontroller/`

The ideal memory controller responds to read/write requests with a fixed
latency and unlimited concurrency. It demonstrates multiple middlewares,
richer Spec/State, and integration with shared storage.

#### 9.2.1 Spec

```go
// comp.go
type Spec struct {
    Width         int    `json:"width"`
    Latency       int    `json:"latency"`
    CacheLineSize int    `json:"cache_line_size"`
    StorageRef    string `json:"storage_ref"`
    AddrConvKind  string `json:"addr_conv_kind"`
}
```

Key design choices:
- `Width` controls how many requests are accepted per tick.
- `Latency` is the fixed response delay in cycles.
- `StorageRef` is a **string ID** referencing shared storage — not a pointer.
- `AddrConvKind` describes the address conversion strategy as a string
  descriptor, resolved to a concrete implementation in the builder.

#### 9.2.2 State

```go
// comp.go
type inflightTransaction struct {
    CycleLeft      int            `json:"cycle_left"`
    Address        uint64         `json:"address"`
    AccessByteSize uint64         `json:"access_byte_size"`
    ReqID          string         `json:"req_id"`
    IsRead         bool           `json:"is_read"`
    Data           []byte         `json:"data,omitempty"`
    DirtyMask      []bool         `json:"dirty_mask,omitempty"`
    Src            sim.RemotePort `json:"src"`
}

type State struct {
    InflightTransactions []inflightTransaction `json:"inflight_transactions"`
    CurrentState         string                `json:"current_state"`
    CurrentCmdID         string                `json:"current_cmd_id"`
    CurrentCmdSrc        sim.RemotePort        `json:"current_cmd_src"`
}
```

Key design choices:
- `inflightTransaction` is a nested struct — allowed in State.
- `Data` is `[]byte` (slice of uint8 primitives) — valid.
- `DirtyMask` is `[]bool` — valid.
- `Src` and `CurrentCmdSrc` are `sim.RemotePort` (string) — not pointers.
- `CurrentState` is a string enum (`"enable"`, `"pause"`, `"drain"`) rather
  than an iota constant — keeps it human-readable in JSON.

#### 9.2.3 Comp (Outer Component)

```go
// comp.go
type Comp struct {
    *modeling.Component[Spec, State]

    topPort          sim.Port
    ctrlPort         sim.Port
    Storage          *mem.Storage
    addressConverter mem.AddressConverter
}
```

Note that `Storage` and `addressConverter` are live objects stored on `Comp`,
**not** in State. They are resolved at build time and cannot be serialized.
The Spec's `StorageRef` provides the string ID for restoring the storage
association after a checkpoint load.

#### 9.2.4 Two Middlewares

**ctrlMiddleware** — handles control commands (enable, pause, drain):

```go
// ctrlmiddleware.go
type ctrlMiddleware struct {
    *Comp
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
    madeProgress = m.handleIncomingCommands() || madeProgress
    madeProgress = m.handleStateUpdate() || madeProgress
    return madeProgress
}
```

This middleware reads control messages from `m.ctrlPort`, updates
`state.CurrentState`, and sends responses back.

**memMiddleware** — handles memory read/write requests:

```go
// memMiddleware.go
type memMiddleware struct {
    *Comp
}

func (m *memMiddleware) Tick() bool {
    madeProgress := false
    madeProgress = m.takeNewReqs() || madeProgress
    madeProgress = m.processCountdowns() || madeProgress
    return madeProgress
}
```

This middleware:
1. Checks `state.CurrentState` — only accepts new requests when `"enable"`
2. Reads up to `spec.Width` requests per tick from `m.topPort`
3. Converts each request into an `inflightTransaction` with
   `CycleLeft = spec.Latency`
4. Decrements countdowns each tick
5. When countdown reaches 0, performs the actual storage read/write and sends
   a response

The two middlewares are registered in order in the builder:

```go
ctrlMiddleware := &ctrlMiddleware{Comp: c}
c.AddMiddleware(ctrlMiddleware)
funcMiddleware := &memMiddleware{Comp: c}
c.AddMiddleware(funcMiddleware)
```

Control middleware runs first, so a drain/pause command takes effect before
the memory middleware processes new requests in the same tick.

#### 9.2.5 Builder

```go
// builder.go
func MakeBuilder() Builder {
    return Builder{
        freq:       1 * sim.GHz,
        capacity:   4 * mem.GB,
        topBufSize: 16,
        spec: &Spec{
            Latency:       100,
            Width:         1,
            CacheLineSize: 64,
        },
    }
}

func (b Builder) Build(name string) *Comp {
    spec := *b.spec
    spec.StorageRef = name

    initialState := State{
        CurrentState: "enable",
    }

    modelComp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(spec).
        Build(name)
    modelComp.SetState(initialState)

    c := &Comp{
        Component:        modelComp,
        addressConverter: b.addressConverter,
    }

    if b.storage == nil {
        c.Storage = mem.NewStorage(b.capacity)
    } else {
        c.Storage = b.storage
    }

    ctrlMiddleware := &ctrlMiddleware{Comp: c}
    c.AddMiddleware(ctrlMiddleware)
    funcMiddleware := &memMiddleware{Comp: c}
    c.AddMiddleware(funcMiddleware)

    c.topPort = b.topPort
    c.topPort.SetComponent(c)
    c.AddPort("Top", c.topPort)
    c.ctrlPort = b.ctrlPort
    c.ctrlPort.SetComponent(c)
    c.AddPort("Control", c.ctrlPort)

    return c
}
```

Notable patterns:
- **Defaults in `MakeBuilder`**: Latency=100, Width=1, CacheLineSize=64,
  capacity=4GB.
- **`StorageRef` is set automatically** to the component name.
- **Storage is created if not provided** — dependency injection with fallback.
- **Initial state sets `CurrentState: "enable"`** — the component starts in
  the enabled mode.

#### 9.2.6 Usage

```go
topPort := sim.NewPort(nil, 16, 16, "MemCtrl.TopPort")
ctrlPort := sim.NewPort(nil, 4, 4, "MemCtrl.CtrlPort")

memCtrl := idealmemcontroller.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithNewStorage(4 * mem.GB).
    WithTopPort(topPort).
    WithCtrlPort(ctrlPort).
    Build("MemCtrl")

// Connect to other components
conn.PlugIn(topPort)
```

---

## Quick-Start Checklist

When creating a new V5 component, follow these steps:

- [ ] **Define messages** — structs embedding `sim.MsgMeta`
- [ ] **Define Spec** — flat struct with primitives only; add `json` tags
- [ ] **Define State** — struct with primitives and nested structs; add `json`
  tags; use string IDs for cross-references
- [ ] **Define Comp** — struct embedding `*modeling.Component[Spec, State]`
  with port fields
- [ ] **Implement middleware(s)** — struct(s) embedding `*Comp`, implementing
  `Tick() bool`
- [ ] **Implement Builder** — `MakeBuilder()`, `With...()` methods,
  `Build(name)` that:
  - Creates `modeling.Component` via inner builder
  - Sets initial state
  - Creates outer Comp
  - Adds middlewares
  - Wires ports (`SetComponent` + `AddPort`)
- [ ] **Validate** — optionally call `ValidateSpec`/`ValidateState` in tests
  or builder
- [ ] **Test** — write unit tests driving behavior via ticks and ports
