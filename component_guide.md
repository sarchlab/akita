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
│  │(immutable│  │(in-place │  │(external │ │
│  │  config) │  │ updated) │  │  I/O)    │ │
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
*current runtime condition*. It is updated in-place each tick (see §2.3).

- Holds in-flight transactions, counters, queues, mode flags, etc.
- Must contain only primitives, **nested structs**, slices/maps of primitives or structs.
- No pointers, interfaces, functions, or channels.
- Cross-references between components use **string IDs**, never pointers.
- Must be JSON-serializable (for checkpoint/restore).

### 1.3 Ports

Ports are the component's communication endpoints. In V5, ports are created
**externally** and injected into the component via the builder. This makes
wiring explicit and decouples port creation from component construction.
Ports are accessed at runtime via `comp.GetPortByName("PortName")`.

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

    spec    S
    current T
    next    T
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
| `GetState() T` | Returns the current state |
| `GetNextState() *T` | Returns a **pointer** to the next state — **write target** |
| `SetNextState(state T)` | Replaces the next state directly |
| `SetState(state T)` | Sets **both** current and next (for initialization and save/load) |
| `Tick() bool` | Assigns current→next, runs middleware pipeline, assigns next→current |
| `AddMiddleware(mw)` | Appends a middleware to the pipeline |
| `AddPort(name, port)` | Registers a port with the component |
| `Name() string` | Returns the component's name |
| `CurrentTime()` | Returns the current simulation time |
| `TickLater()` | Schedules a tick on the next cycle |
| `SaveState(w)` | Serializes spec + current state to JSON |
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

### 2.3 In-Place State Update

`Component[S, T]` uses **in-place state update**. The `current` and `next`
fields hold the same state value during a tick:

- **`current`** — the state visible via `GetState()`
- **`next`** — the state accessible via `GetNextState()` (a pointer for mutation)

The `Tick()` method implements the update cycle:

```go
// From v5/modeling/component.go
func (c *Component[S, T]) Tick() bool {
    c.next = c.current                        // 1. Assign current → next
    madeProgress := c.MiddlewareHolder.Tick() // 2. Run middleware pipeline
    c.current = c.next                        // 3. Assign next → current

    return madeProgress
}
```

1. **Before each tick:** `current` is assigned to `next` (shallow copy).
2. **During the tick:** Middlewares read from `GetState()` and write to
   `GetNextState()`. Since both refer to the same underlying data, reads
   and writes are interchangeable.
3. **After the tick:** `next` is assigned back to `current`.

**Key rules:**
- **Write through `GetNextState()`** — it returns a pointer, enabling direct
  mutation. `GetState()` returns a value copy of the struct (but slices and
  maps inside share underlying data).
- State types must be JSON-serializable (for checkpoint/restore) with no
  pointers, interfaces, functions, or channels.

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
4. `stateutil.Buffer[T]` and `stateutil.Pipeline[T]` are allowed — they are
   concrete value types with exported JSON-tagged fields, so they behave like
   nested structs and serialize automatically (see §4A).
5. Nested structs must themselves follow the same rules recursively — no
   pointers, interfaces, functions, or channels at any nesting depth.
6. Cross-references to other components must use **string IDs**, not pointers.
7. All fields should have `json:"..."` tags for serialization.

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
| `stateutil.Buffer[T]` / `Pipeline[T]` | ❌ | ✅ |
| Pointers | ❌ | ❌ |
| Interfaces | ❌ | ❌ |
| Functions | ❌ | ❌ |
| Channels | ❌ | ❌ |

---

## 4A. State Primitives: `stateutil.Buffer[T]` and `Pipeline[T]`

> Source: `v5/stateutil/buffer.go`, `v5/stateutil/pipeline.go`

### What stateutil Provides

The `stateutil` package provides two generic, JSON-serializable container types
designed for use inside State structs:

- **`stateutil.Buffer[T]`** — a generic FIFO queue that satisfies the
  `queueing.Buffer` interface. It is a concrete value type (not an interface)
  with exported, JSON-tagged fields.
- **`stateutil.Pipeline[T]`** — a generic multi-lane, multi-stage pipeline.
  It is also a concrete value type with exported, JSON-tagged fields.

Both types serialize automatically as part of State JSON marshaling — **no
custom `GetState`/`SetState` is needed**.

### Why `Buffer[T]`, Not the Old `queueing.Buffer`

In earlier versions of Akita, queues were typically `queueing.Buffer` — an
**interface** type. Interfaces are not JSON-serializable and cannot be stored
in State (they violate the "no interfaces, no pointers" rule).

`stateutil.Buffer[T]` solves this by being a **concrete value type** with
exported fields:

```go
// From v5/stateutil/buffer.go
type Buffer[T any] struct {
    sim.HookableBase `json:"-"`

    BufferName string `json:"buffer_name"`
    Cap        int    `json:"cap"`
    Elements   []T    `json:"elements"`
}
```

- `HookableBase` is excluded from JSON via `json:"-"`.
- `BufferName`, `Cap`, and `Elements` are exported with JSON tags —
  they serialize automatically.
- The type parameter `T` must itself be JSON-serializable (primitives,
  structs, slices — no pointers or interfaces).

### Embedding in State Structs

Embed `Buffer[T]` and `Pipeline[T]` directly in your State struct with JSON
tags, just like any other nested struct:

```go
// From v5/mem/cache/writethroughcache/cache.go
type State struct {
    // ...
    DirBuf        stateutil.Buffer[int]     `json:"dir_buf"`
    BankBufs      []stateutil.Buffer[int]   `json:"bank_bufs"`
    DirPipeline   stateutil.Pipeline[int]   `json:"dir_pipeline"`
    DirPostBuf    stateutil.Buffer[int]     `json:"dir_post_buf"`
    BankPipelines []stateutil.Pipeline[int] `json:"bank_pipelines"`
    BankPostBufs  []stateutil.Buffer[int]   `json:"bank_post_bufs"`
    // ...
}
```

Slices of buffers and pipelines are also valid (e.g., `[]stateutil.Buffer[int]`
for per-bank queues).

The writeback cache uses the same pattern with more buffers:

```go
// From v5/mem/cache/writeback/writebackcache.go
type State struct {
    // ...
    DirStageBuf           stateutil.Buffer[int]     `json:"dir_stage_buf"`
    DirToBankBufs         []stateutil.Buffer[int]   `json:"dir_to_bank_bufs"`
    WriteBufferToBankBufs []stateutil.Buffer[int]   `json:"write_buffer_to_bank_bufs"`
    MSHRStageBuf          stateutil.Buffer[int]     `json:"mshr_stage_buf"`
    WriteBufferBuf        stateutil.Buffer[int]     `json:"write_buffer_buf"`

    DirPipeline        stateutil.Pipeline[int] `json:"dir_pipeline"`
    DirPostPipelineBuf stateutil.Buffer[int]   `json:"dir_post_pipeline_buf"`

    BankPipelines        []stateutil.Pipeline[int] `json:"bank_pipelines"`
    BankPostPipelineBufs []stateutil.Buffer[int]   `json:"bank_post_pipeline_bufs"`
    // ...
}
```

**Note:** The type parameter is typically `int` — representing an index into a
transaction slice in State (see §4B for the flat transaction pattern).

### `Buffer[T]` Key Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `CanPush` | `() bool` | Returns `true` if the buffer has room |
| `Push` | `(e interface{})` | Adds an element (satisfies `queueing.Buffer`) |
| `PushTyped` | `(e T)` | Adds an element with the concrete type |
| `Pop` | `() interface{}` | Removes and returns the front element |
| `PopTyped` | `() (T, bool)` | Removes and returns the front element with concrete type |
| `Peek` | `() interface{}` | Returns the front element without removing |
| `Clear` | `()` | Removes all elements |
| `Size` | `() int` | Returns the number of elements |
| `Capacity` | `() int` | Returns the buffer's capacity |
| `Name` | `() string` | Returns the buffer's name |

### `Pipeline[T]` Structure

```go
// From v5/stateutil/pipeline.go
type PipelineStage[T any] struct {
    Lane      int `json:"lane"`
    Stage     int `json:"stage"`
    Item      T   `json:"item"`
    CycleLeft int `json:"cycle_left"`
}

type Pipeline[T any] struct {
    Width     int                `json:"width"`
    NumStages int                `json:"num_stages"`
    Stages    []PipelineStage[T] `json:"stages"`
}
```

- `Width` — the number of lanes (items that can be at the same stage
  simultaneously).
- `NumStages` — the total number of stages. An item starts at stage 0 with
  `CycleLeft = NumStages - 1`.
- `Stages` — a flat slice of all active items in the pipeline. Each
  `PipelineStage` records the item's current lane, stage, and remaining
  cycles.

### `Pipeline[T]` Key Methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `CanAccept` | `() bool` | Returns `true` if there is a free lane at stage 0 |
| `Accept` | `(item T)` | Inserts an item into the first stage |
| `Tick` | `(postBuf *Buffer[T]) bool` | Advances the pipeline by one cycle; completed items are pushed into `postBuf` |
| `TickFunc` | `(accept func(T) bool) bool` | Like `Tick`, but uses a custom accept function for completed items |

---

## 4B. Flat Transaction Pattern

### The Problem

Messages like `*mem.ReadReq` and `*mem.WriteReq` are pointer types. Storing
them in State violates the "no pointers, no interfaces" rule and breaks
JSON serialization. In pre-V5 code, transaction structs commonly held fields
like:

```go
// ❌ Old pattern — pointer fields, NOT serializable
type transaction struct {
    Read          *mem.ReadReq
    Write         *mem.WriteReq
    ReadToBottom  *mem.ReadReq
    WriteToBottom *mem.WriteReq
    PreCoalesce   []*transaction
}
```

This cannot be stored in State because:
- `*mem.ReadReq` and `*mem.WriteReq` are pointers
- `[]*transaction` is a slice of pointers
- None of these types are JSON-serializable

### The Solution: Flat Value Fields

Replace each pointer field with flat value fields that capture exactly the
data needed. Use a `bool` flag to indicate whether the field is populated:

```go
// ✅ New pattern — flat fields, fully JSON-serializable
// From v5/mem/cache/writethroughcache/transaction.go
type transactionState struct {
    ID string `json:"id"`

    // Read request fields (flattened from *mem.ReadReq)
    HasRead            bool        `json:"has_read"`
    ReadMeta           sim.MsgMeta `json:"read_meta"`
    ReadAddress        uint64      `json:"read_address"`
    ReadAccessByteSize uint64      `json:"read_access_byte_size"`
    ReadPID            vm.PID      `json:"read_pid"`

    // Write request fields (flattened from *mem.WriteReq)
    HasWrite       bool        `json:"has_write"`
    WriteMeta      sim.MsgMeta `json:"write_meta"`
    WriteAddress   uint64      `json:"write_address"`
    WriteData      []byte      `json:"write_data"`
    WriteDirtyMask []bool      `json:"write_dirty_mask"`
    WritePID       vm.PID      `json:"write_pid"`

    // Pre-coalesce transaction indices (replaces []*transaction)
    PreCoalesceTransIdxs []int `json:"pre_coalesce_trans_idxs"`

    // ... additional flat fields ...
}
```

### Key Principles

1. **`Has*` booleans replace nil checks.** Instead of `if t.Read != nil`,
   use `if t.HasRead`.

2. **Cross-references use indices, not pointers.** Instead of
   `[]*transactionState`, use `[]int` — indices into the transaction slice
   in State:

    ```go
    // From v5/mem/cache/writethroughcache/transaction.go
    PreCoalesceTransIdxs []int `json:"pre_coalesce_trans_idxs"`
    ```

    ```go
    // From v5/mem/cache/writeback/transaction.go
    MSHRTransactionIndices []int `json:"mshr_transaction_indices"`
    ```

3. **Messages are reconstructed at send boundaries.** Concrete message types
   (`ReadReq`, `WriteReq`, etc.) are built fresh from flat fields when they
   need to be sent over a port — they are never stored in State:

    ```go
    // Example: reconstructing a ReadReq from flat fields at send time
    req := &mem.ReadReq{}
    req.MsgMeta = trans.ReadToBottomMeta
    req.Address = trans.ReadAddress
    req.AccessByteSize = trans.ReadAccessByteSize
    req.PID = trans.ReadToBottomPID
    err := bottomPort.Send(req)
    ```

4. **`sim.MsgMeta` is a value type**, not a pointer — it is safe to store
   directly in State. It holds `ID`, `Src`, `Dst`, `RspTo`, and other
   routing metadata as string-typed fields.

### Before and After Comparison

**Before (pointer-based — ❌ cannot go in State):**

```go
type transaction struct {
    Read          *mem.ReadReq          // pointer — not serializable
    Write         *mem.WriteReq         // pointer — not serializable
    PreCoalesce   []*transaction        // pointer slice — not serializable
    Block         *cache.Block          // pointer — not serializable
}
```

**After (flat — ✅ fully serializable):**

```go
// From v5/mem/cache/writethroughcache/transaction.go
type transactionState struct {
    HasRead            bool        `json:"has_read"`
    ReadMeta           sim.MsgMeta `json:"read_meta"`
    ReadAddress        uint64      `json:"read_address"`
    ReadAccessByteSize uint64      `json:"read_access_byte_size"`
    ReadPID            vm.PID      `json:"read_pid"`

    HasWrite       bool        `json:"has_write"`
    WriteMeta      sim.MsgMeta `json:"write_meta"`
    WriteAddress   uint64      `json:"write_address"`
    WriteData      []byte      `json:"write_data"`
    WriteDirtyMask []bool      `json:"write_dirty_mask"`
    WritePID       vm.PID      `json:"write_pid"`

    PreCoalesceTransIdxs []int `json:"pre_coalesce_trans_idxs"`

    BlockSetID int  `json:"block_set_id"`
    BlockWayID int  `json:"block_way_id"`
    HasBlock   bool `json:"has_block"`
}
```

### Result

With flat transactions and stateutil containers, **no custom
`GetState`/`SetState` logic is needed on middlewares.** The
`modeling.Component` handles serialization automatically — `SaveState` and
`LoadState` marshal/unmarshal the entire State (including all transactions,
buffers, and pipelines) via standard JSON.

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

### 5.2 Middleware Structure — No Comp Wrapper

The canonical pattern is for each middleware to hold a pointer to
`*modeling.Component[Spec, State]` **directly**, not to an outer `*Comp`
wrapper:

```go
// From v5/examples/tickingping/comp.go
type sendMW struct {
    comp *modeling.Component[Spec, State]
}
```

This gives the middleware access to Spec, State, and Ports via the component's
methods.

### 5.3 Port Access via Helper Functions

Ports are **not** stored as fields on the middleware or on a wrapper struct.
Instead, use package-level helper functions or methods that call
`comp.GetPortByName()`:

```go
// From v5/examples/tickingping/comp.go
func outPort(comp *modeling.Component[Spec, State]) sim.Port {
    return comp.GetPortByName("Out")
}
```

For middlewares with a receiver, a method-style helper also works:

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
func (m *memMiddleware) topPort() sim.Port {
    return m.comp.GetPortByName("Top")
}
```

### 5.4 Tracing Pattern

Middlewares do **not** implement `NamedHookable` themselves. Instead,
middlewares pass `m.comp` directly to tracing functions. The component
itself is the `NamedHookable` — it already implements the interface via
`sim.ComponentBase`.

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
tracing.TraceReqReceive(msg, m.comp)
```

When completing a traced request, the middleware constructs the task ID and
calls `tracing.EndTask` with `m.comp`:

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
func (m *memMiddleware) traceReqComplete(reqID string) {
    taskID := fmt.Sprintf("%s@%s", reqID, m.comp.Name())
    tracing.EndTask(taskID, m.comp)
}
```

This keeps tracing simple: the component is the hook domain, and
middlewares just forward `m.comp` to any tracing call that needs a
`NamedHookable`.

### 5.5 Tick() Logic — GetState/GetNextState Pattern

Inside a middleware's methods, read from `GetState()` and write to
`GetNextState()`. With in-place update semantics both refer to the same
underlying data, but the convention keeps code readable:

```go
// From v5/examples/tickingping/comp.go — sendMW.sendRsp()
func (m *sendMW) sendRsp() bool {
    state := m.comp.GetState()   // current state

    if len(state.CurrentTransactions) == 0 {
        return false
    }

    trans := state.CurrentTransactions[0]
    if trans.CycleLeft > 0 {
        return false
    }

    rsp := &PingRsp{
        MsgMeta: sim.MsgMeta{
            ID:    sim.GetIDGenerator().Generate(),
            Src:   outPort(m.comp).AsRemote(),
            Dst:   trans.ReqSrc,
            RspTo: trans.ReqID,
        },
        SeqID: trans.SeqID,
    }

    err := outPort(m.comp).Send(rsp)
    if err != nil {
        return false
    }

    next := m.comp.GetNextState()                    // writable pointer
    next.CurrentTransactions = next.CurrentTransactions[1:]  // mutate state directly

    return true
}
```

**Key pattern:** Read conditions from `GetState()`, then mutate via `GetNextState()`. Both refer to the same underlying data with in-place update semantics.

A typical `Tick()` method orchestrates multiple sub-operations:

```go
// From v5/examples/tickingping/comp.go
func (m *sendMW) Tick() bool {
    madeProgress := false

    madeProgress = m.sendRsp() || madeProgress
    madeProgress = m.sendPing() || madeProgress

    return madeProgress
}
```

Each sub-operation:
1. Reads state via `m.comp.GetState()` to check conditions
2. Does the work (send messages, etc.)
3. Writes mutations via `m.comp.GetNextState()`
4. Returns `true` if it did something, `false` otherwise

### 5.6 Multiple Middlewares

A component can have multiple middlewares, each responsible for a different
concern. The middlewares are called in the order they were added.

**tickingping** has two middlewares:

1. **`sendMW`** — handles sending responses (`sendRsp`) and ping requests (`sendPing`)
2. **`receiveProcessMW`** — handles counting down timers (`countDown`) and processing incoming messages (`processInput`)

```go
// From v5/examples/tickingping/builder.go
comp.AddMiddleware(&sendMW{comp: comp})
comp.AddMiddleware(&receiveProcessMW{comp: comp})
```

**idealmemcontroller** also has two middlewares:

1. **`ctrlMiddleware`** — handles enable/pause/drain control commands
2. **`memMiddleware`** — handles memory read/write requests

```go
// From v5/mem/idealmemcontroller/builder.go
ctrlMW := &ctrlMiddleware{comp: modelComp}
modelComp.AddMiddleware(ctrlMW)
memMW := &memMiddleware{comp: modelComp, storage: c.storage}
modelComp.AddMiddleware(memMW)
```

Control middleware runs first, so a drain/pause command takes effect before
the memory middleware processes new requests in the same tick.

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
// From v5/examples/tickingping/builder.go
func (b Builder) WithOutPort(port sim.Port) Builder {
    b.outPort = port
    return b
}
```

### 6.4 SetComponent and AddPort

After building a component, each port must be associated with it via
`SetComponent` and registered by name via `AddPort`. This is done inside the
builder's `Build` method:

```go
// From v5/examples/tickingping/builder.go
b.outPort.SetComponent(comp)
comp.AddPort("Out", b.outPort)
```

`SetComponent` is critical because:
- It lets the port call `NotifyRecv` and `NotifyPortFree` on the owning
  component, which triggers the tick-based lifecycle
- Without it, the component won't wake up when messages arrive

At runtime, middlewares retrieve ports by name:

```go
port := comp.GetPortByName("Out")
```

### 6.5 Connecting Ports

After building components, connect their ports using a connection:

```go
conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Conn")

conn.PlugIn(agentA.GetPortByName("Out"))
conn.PlugIn(agentB.GetPortByName("Out"))
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
conn.PlugIn(otherComponent.GetPortByName("Top"))
```

---

## 7. Save/Load via SaveState/LoadState

### 7.1 How It Works

`Component[S, T]` provides built-in checkpoint support through JSON
serialization of both Spec and State:

```go
// From v5/modeling/saveload.go

// SaveState marshals spec + current state as JSON to a writer.
func (c *Component[S, T]) SaveState(w io.Writer) error

// LoadState reads JSON from a reader and restores spec + state.
// The loaded state is written to both current and next.
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

The default pattern returns `*modeling.Component[Spec, State]` directly —
**no outer Comp wrapper** is needed unless the component must implement
additional interfaces (see §8.4):

```go
package mycomponent

import (
    "github.com/sarchlab/akita/v5/modeling"
    "github.com/sarchlab/akita/v5/sim"
)

type Builder struct {
    engine  sim.Engine
    freq    sim.Freq
    port    sim.Port
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

func (b Builder) WithPort(port sim.Port) Builder {
    b.port = port
    return b
}

// Build creates the component — returns *modeling.Component directly.
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
    comp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(Spec{}).
        Build(name)
    comp.SetState(State{})

    comp.AddMiddleware(&myMiddleware{comp: comp})

    b.port.SetComponent(comp)
    comp.AddPort("Main", b.port)

    return comp
}
```

This pattern is taken directly from the tickingping builder:

```go
// From v5/examples/tickingping/builder.go
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
    comp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(Spec{}).
        Build(name)
    comp.SetState(State{})

    comp.AddMiddleware(&sendMW{comp: comp})
    comp.AddMiddleware(&receiveProcessMW{comp: comp})

    b.outPort.SetComponent(comp)
    comp.AddPort("Out", b.outPort)

    return comp
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
- **Middlewares hold `comp *modeling.Component[Spec, State]`** — not a wrapper type.

### 8.4 When to Use a Thin Comp Wrapper

Use a thin `Comp` wrapper **only** when the component needs to implement
additional interfaces (e.g., `StorageOwner`). The wrapper embeds
`*modeling.Component[Spec, State]` and adds only the extra fields/methods
needed:

```go
// From v5/mem/idealmemcontroller/comp.go
type Comp struct {
    *modeling.Component[Spec, State]

    storage *mem.Storage
}

func (c *Comp) GetStorage() *mem.Storage {
    return c.storage
}

func (c *Comp) StorageName() string {
    return c.GetSpec().StorageRef
}
```

**Important:** Even when a `Comp` wrapper exists, the middlewares still hold
`comp *modeling.Component[Spec, State]`, **not** `*Comp`:

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
type memMiddleware struct {
    comp    *modeling.Component[Spec, State]
    storage *mem.Storage
}
```

When the middleware needs access to non-state resources (like `*mem.Storage`),
those are passed directly to the middleware struct — not accessed through
a `Comp` wrapper.

---

## 9. Complete Walkthroughs

### 9.1 tickingping — Simple Component (No Comp Wrapper)

> Source: `v5/examples/tickingping/`

The `tickingping` component sends ping requests and responds to them. Two
instances are connected and exchange pings, measuring round-trip time. This
is the canonical example — it uses **no Comp wrapper**.

#### 9.1.1 Messages

```go
// From v5/examples/tickingping/comp.go
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
// From v5/examples/tickingping/comp.go
type Spec struct{}
```

This component has no configurable parameters, so the Spec is an empty struct.
It still satisfies the Spec constraints (empty struct = valid).

#### 9.1.3 State

```go
// From v5/examples/tickingping/comp.go
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

#### 9.1.4 Port Helper

There is no Comp wrapper. Ports are accessed via a package-level helper function:

```go
// From v5/examples/tickingping/comp.go
func outPort(comp *modeling.Component[Spec, State]) sim.Port {
    return comp.GetPortByName("Out")
}
```

#### 9.1.5 Middlewares

The component has two middlewares: `sendMW` and `receiveProcessMW`.

**sendMW** — handles sending responses and ping requests:

```go
// From v5/examples/tickingping/comp.go
type sendMW struct {
    comp *modeling.Component[Spec, State]
}

func (m *sendMW) Tick() bool {
    madeProgress := false

    madeProgress = m.sendRsp() || madeProgress
    madeProgress = m.sendPing() || madeProgress

    return madeProgress
}
```

The `sendPing` method demonstrates the GetState/GetNextState pattern:

```go
// From v5/examples/tickingping/comp.go
func (m *sendMW) sendPing() bool {
    state := m.comp.GetState()              // current state

    if state.NumPingNeedToSend == 0 {
        return false
    }

    pingMsg := &PingReq{
        MsgMeta: sim.MsgMeta{
            ID:  sim.GetIDGenerator().Generate(),
            Src: outPort(m.comp).AsRemote(),
            Dst: state.PingDst,
        },
        SeqID: state.NextSeqID,
    }

    err := outPort(m.comp).Send(pingMsg)
    if err != nil {
        return false
    }

    next := m.comp.GetNextState()           // writable pointer (same data)
    next.StartTimes = append(next.StartTimes, float64(m.comp.CurrentTime()))
    next.NumPingNeedToSend--
    next.NextSeqID++

    return true
}
```

**receiveProcessMW** — handles counting down timers and processing incoming messages:

```go
// From v5/examples/tickingping/comp.go
type receiveProcessMW struct {
    comp *modeling.Component[Spec, State]
}

func (m *receiveProcessMW) Tick() bool {
    madeProgress := false

    madeProgress = m.countDown() || madeProgress
    madeProgress = m.processInput() || madeProgress

    return madeProgress
}
```

The `processInput` method shows message dispatching with peek-then-retrieve:

```go
// From v5/examples/tickingping/comp.go
func (m *receiveProcessMW) processInput() bool {
    msgI := outPort(m.comp).PeekIncoming()
    if msgI == nil {
        return false
    }

    switch msg := msgI.(type) {
    case *PingReq:
        m.processingPingReq(msg)
    case *PingRsp:
        m.processingPingRsp(msg)
    default:
        panic("unknown message type")
    }

    return true
}

func (m *receiveProcessMW) processingPingReq(msg *PingReq) {
    next := m.comp.GetNextState()

    trans := pingTransactionState{
        SeqID:     msg.SeqID,
        CycleLeft: 2,
        ReqID:     msg.ID,
        ReqSrc:    msg.Src,
    }
    next.CurrentTransactions = append(next.CurrentTransactions, trans)

    outPort(m.comp).RetrieveIncoming()
}
```

Note the peek-then-retrieve pattern: `PeekIncoming()` checks without
consuming; `RetrieveIncoming()` removes the message from the buffer.

#### 9.1.6 Builder

```go
// From v5/examples/tickingping/builder.go
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

func (b Builder) Build(name string) *modeling.Component[Spec, State] {
    comp := modeling.NewBuilder[Spec, State]().
        WithEngine(b.engine).
        WithFreq(b.freq).
        WithSpec(Spec{}).
        Build(name)
    comp.SetState(State{})

    comp.AddMiddleware(&sendMW{comp: comp})
    comp.AddMiddleware(&receiveProcessMW{comp: comp})

    b.outPort.SetComponent(comp)
    comp.AddPort("Out", b.outPort)

    return comp
}
```

The builder returns `*modeling.Component[Spec, State]` directly — no
`Comp` wrapper needed.

#### 9.1.7 Usage

```go
// From v5/examples/tickingping/example_test.go
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
    conn := directconnection.
        MakeBuilder().
        WithEngine(engine).
        WithFreq(1 * sim.GHz).
        Build("Conn")

    conn.PlugIn(agentA.GetPortByName("Out"))
    conn.PlugIn(agentB.GetPortByName("Out"))

    state := agentA.GetState()
    state.PingDst = agentB.GetPortByName("Out").AsRemote()
    state.NumPingNeedToSend = 2
    agentA.SetState(state)

    agentA.TickLater()

    err := engine.Run()
    if err != nil {
        panic(err)
    }

    // Output:
    // Ping 0, 5.00
    // Ping 1, 5.00
}
```

Note that ports are accessed via `agentA.GetPortByName("Out")`, not via
a field on a Comp wrapper.

---

### 9.2 idealmemcontroller — Component with Thin Comp Wrapper

> Source: `v5/mem/idealmemcontroller/`

The ideal memory controller responds to read/write requests with a fixed
latency and unlimited concurrency. It demonstrates multiple middlewares,
richer Spec/State, integration with shared storage, and the **thin Comp
wrapper** pattern (needed because the component implements the
`StorageOwner` interface).

#### 9.2.1 Spec

```go
// From v5/mem/idealmemcontroller/comp.go
type Spec struct {
    Width         int    `json:"width"`
    Latency       int    `json:"latency"`
    CacheLineSize int    `json:"cache_line_size"`
    StorageRef    string `json:"storage_ref"`
    AddrConvKind  string `json:"addr_conv_kind"`

    AddrInterleavingSize    uint64 `json:"addr_interleaving_size"`
    AddrTotalNumOfElements  int    `json:"addr_total_num_of_elements"`
    AddrCurrentElementIndex int    `json:"addr_current_element_index"`
    AddrOffset              uint64 `json:"addr_offset"`
}
```

Key design choices:
- `Width` controls how many requests are accepted per tick.
- `Latency` is the fixed response delay in cycles.
- `StorageRef` is a **string ID** referencing shared storage — not a pointer.
- `AddrConvKind` describes the address conversion strategy as a string
  descriptor. The address conversion parameters (`AddrInterleavingSize`, etc.)
  are stored as primitives in Spec, resolved from an `AddressConverter`
  interface in the builder.

#### 9.2.2 State

```go
// From v5/mem/idealmemcontroller/comp.go
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

#### 9.2.3 Comp (Thin Wrapper for StorageOwner)

```go
// From v5/mem/idealmemcontroller/comp.go
type Comp struct {
    *modeling.Component[Spec, State]

    storage *mem.Storage
}

func (c *Comp) GetStorage() *mem.Storage {
    return c.storage
}

func (c *Comp) StorageName() string {
    return c.GetSpec().StorageRef
}
```

The `Comp` wrapper exists **only** because the component needs to implement
the `StorageOwner` interface (providing `GetStorage()` and `StorageName()`).
The `*mem.Storage` is a live object that cannot be serialized — it lives on
`Comp`, not in State. The Spec's `StorageRef` provides the string ID for
restoring the storage association after a checkpoint load.

#### 9.2.4 Two Middlewares

Both middlewares hold `comp *modeling.Component[Spec, State]`, **not** `*Comp`.

**ctrlMiddleware** — handles control commands (enable, pause, drain):

```go
// From v5/mem/idealmemcontroller/ctrlmiddleware.go
type ctrlMiddleware struct {
    comp *modeling.Component[Spec, State]
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
    madeProgress = m.handleIncomingCommands() || madeProgress
    madeProgress = m.handleStateUpdate() || madeProgress
    return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() sim.Port {
    return m.comp.GetPortByName("Control")
}
```

The `handleDrainState` method shows reading from `GetState()` and writing
to `GetNextState()`:

```go
// From v5/mem/idealmemcontroller/ctrlmiddleware.go
func (m *ctrlMiddleware) handleDrainState() bool {
    state := m.comp.GetState()
    if len(state.InflightTransactions) != 0 {
        return false
    }

    rsp := &mem.ControlMsgRsp{}
    rsp.ID = sim.GetIDGenerator().Generate()
    rsp.Src = m.ctrlPort().AsRemote()
    rsp.Dst = state.CurrentCmdSrc
    rsp.RspTo = state.CurrentCmdID
    rsp.TrafficClass = "mem.ControlMsgRsp"

    err := m.ctrlPort().Send(rsp)
    if err != nil {
        return false
    }

    nextState := m.comp.GetNextState()
    nextState.CurrentState = "pause"

    return true
}
```

**memMiddleware** — handles memory read/write requests. Note that it holds
both `comp` and `storage` directly:

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
type memMiddleware struct {
    comp    *modeling.Component[Spec, State]
    storage *mem.Storage
}

func (m *memMiddleware) topPort() sim.Port {
    return m.comp.GetPortByName("Top")
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
2. Reads up to `spec.Width` requests per tick from `topPort()`
3. Converts each request into an `inflightTransaction` with
   `CycleLeft = spec.Latency`
4. Decrements countdowns each tick
5. When countdown reaches 0, performs the actual storage read/write and sends
   a response

The `takeNewReqs` method demonstrates using both `GetState()` and
`GetSpec()` for reads, and `GetNextState()` for writes:

```go
// From v5/mem/idealmemcontroller/memMiddleware.go
func (m *memMiddleware) takeNewReqs() (madeProgress bool) {
    state := m.comp.GetState()
    if state.CurrentState != "enable" {
        return false
    }

    spec := m.comp.GetSpec()

    for i := 0; i < spec.Width; i++ {
        msgI := m.topPort().RetrieveIncoming()
        if msgI == nil {
            break
        }

        msg := msgI.(sim.Msg)
        tracing.TraceReqReceive(msg, m.comp)

        tx := m.msgToInflightTransaction(msg)

        nextState := m.comp.GetNextState()
        nextState.InflightTransactions = append(
            nextState.InflightTransactions, tx)
        madeProgress = true
    }

    return madeProgress
}
```

#### 9.2.5 Builder

```go
// From v5/mem/idealmemcontroller/builder.go
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

    var storage *mem.Storage
    if b.storage == nil {
        storage = mem.NewStorage(b.capacity)
    } else {
        storage = b.storage
    }

    c := &Comp{
        Component: modelComp,
        storage:   storage,
    }

    ctrlMW := &ctrlMiddleware{comp: modelComp}
    modelComp.AddMiddleware(ctrlMW)
    memMW := &memMiddleware{comp: modelComp, storage: c.storage}
    modelComp.AddMiddleware(memMW)

    b.topPort.SetComponent(c)
    modelComp.AddPort("Top", b.topPort)
    b.ctrlPort.SetComponent(c)
    modelComp.AddPort("Control", b.ctrlPort)

    return c
}
```

Notable patterns:
- **`Build` returns `*Comp`** — because the wrapper is needed for `StorageOwner`.
- **Defaults in `MakeBuilder`**: Latency=100, Width=1, CacheLineSize=64,
  capacity=4GB.
- **`StorageRef` is set automatically** to the component name.
- **Storage is created if not provided** — dependency injection with fallback.
- **Initial state sets `CurrentState: "enable"`** — the component starts in
  the enabled mode.
- **`SetComponent(c)` uses the `*Comp` wrapper** — so `NotifyRecv`/`NotifyPortFree`
  go to the right object. But `AddPort` and `AddMiddleware` are called on
  `modelComp`.
- **Middlewares receive `modelComp`** (the `*modeling.Component`), not `c`
  (the `*Comp` wrapper).

---

## 10. No-Dependency Philosophy

V5 components follow a no-external-dependency philosophy:

- **Inline all logic in middleware** — don't create separate dependency
  interfaces (no `VictimFinder`, `Directory`, `MSHR` interfaces).
- **Store port names in Spec** or use string constants — resolve ports via
  `GetPortByName()`.
- **Use pure state functions** — address conversion in idealmemcontroller
  is a package-level function `convertAddress(spec, addr)` that reads
  parameters from Spec, not from an interface.
- **Pass non-state resources directly to middleware** — e.g., `*mem.Storage`
  is a field on `memMiddleware`, not accessed through an interface or wrapper.
- This keeps components self-contained, testable, and serializable.

---

## Quick-Start Checklist

When creating a new V5 component, follow these steps:

- [ ] **Define messages** — structs embedding `sim.MsgMeta`
- [ ] **Define Spec** — flat struct with primitives only; add `json` tags
- [ ] **Define State** — struct with primitives and nested structs; add `json`
  tags; use string IDs for cross-references
- [ ] **Use `stateutil.Buffer[T]` and `Pipeline[T]`** for queues and pipelines
  in State — they are JSON-serializable value types (see §4A); use `int`
  indices as the type parameter to reference transactions in a flat slice
- [ ] **Flatten transaction structs** — replace pointer-to-message fields with
  flat value fields (`Has*` booleans + individual primitive/struct fields);
  use `[]int` indices instead of pointer slices (see §4B)
- [ ] **Implement middleware(s)** — struct(s) holding
  `comp *modeling.Component[Spec, State]`, implementing `Tick() bool`
- [ ] **For tracing, pass `m.comp` to tracing functions** (e.g.
  `tracing.TraceReqReceive`, `tracing.EndTask`) — middlewares do not
  implement NamedHookable themselves
- [ ] **Add port helper(s)** — package-level functions or methods calling
  `comp.GetPortByName("...")`
- [ ] **Implement Builder** — `MakeBuilder()`, `With...()` methods,
  `Build(name)` that:
  - Creates `modeling.Component` via inner builder
  - Sets initial state via `SetState()`
  - Adds middlewares (passing `comp` directly)
  - Wires ports (`SetComponent` + `AddPort`)
  - Returns `*modeling.Component[Spec, State]` by default
- [ ] **Use thin Comp wrapper only if needed** — when the component must
  implement extra interfaces (e.g., `StorageOwner`)
- [ ] **Validate** — optionally call `ValidateSpec`/`ValidateState` in tests
  or builder
- [ ] **Test** — write unit tests driving behavior via ticks and ports
