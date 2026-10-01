# writeback — Write-Back Cache Component

Package `writeback` provides a configurable write-back cache for the Akita
simulation framework. Dirty cache lines are written to lower-level memory only
on eviction, reducing write traffic through the memory hierarchy.

## How It Works

The cache is organized as a pipeline of stages, each executing every tick:

```
TopPort ──► TopParser ──► DirectoryStage ──► BankStage ──► (response)
                              │                   ▲
                              ▼                   │
                          MSHRStage ◄── WriteBufferStage ──► BottomPort
```

| Stage | Role |
|---|---|
| **TopParser** | Receives `memprotocol.ReadReq` and `memprotocol.WriteReq` from the top port, creates internal transactions, and pushes them into the directory stage buffer. |
| **DirectoryStage** | Looks up the set-associative directory. On a hit, routes to the bank stage. On a miss, allocates an MSHR entry and sends the transaction to the write buffer for fetching/eviction. |
| **BankStage** | Performs the actual data read/write through a latency pipeline. Handles hits, evictions, and fetched-data writes. Sends responses back through the top port. |
| **MSHRStage** | Processes completed MSHR entries when fetched data returns. Replays all waiting transactions that targeted the same cache line. |
| **WriteBufferStage** | Manages outstanding fetches and evictions to/from lower-level memory via the bottom port. Tracks inflight fetch and eviction counts. |

The cache's behavior is three middlewares, run in this order every cycle:

- `Ctrl` handles the control commands other than Flush received on the
  `Control` port: Pause, Drain, Enable, Reset, and Invalidate.
- `Flusher` handles Flush: once the cache is paused, it writes the dirty blocks
  that match the request's address/PID filter back to lower memory and marks
  them clean.
- `Pipeline` runs the data pipeline above; it does nothing while the cache is
  paused.

## Key Types

- `Spec` — immutable configuration: frequency, geometry, latencies, MSHR/buffer
  capacities, inflight limits, and address mapping. Port buffer sizes are chosen
  by the system builder when it creates the port instances, not in the spec.
- `State` — mutable runtime data: the directory state, MSHR state, all
  inter-stage buffers and pipelines, the transaction list, and inflight
  counters. Fully JSON-serializable, as required by the `State` constraint.
- `Resources` — shared wiring; holds the backing `*mem.Storage` (required) and
  the address-to-port mapping used to route fetches/evictions to lower memory
  (supply either `AddressToPortMapper` or `RemotePorts`).
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl`, `Flusher`, and `Pipeline`, run in that order every
  cycle.
- `Comp` — `ticking.Component[Spec, State, Resources, Ports, Middlewares]`, a
  ticking component.

```go
type Spec struct {
    Freq                timing.Freq // Operating frequency
    NumReqPerCycle      int         // Requests processed per cycle
    Log2BlockSize       uint64      // Cache line size = 1 << Log2BlockSize
    BankLatency         int         // Cycles per bank read/write
    DirLatency          int         // Cycles for directory lookup
    WayAssociativity    int         // Ways per set
    NumBanks            int         // Number of banks (below 1 means 1)
    NumMSHREntry        int         // MSHR capacity
    TotalByteSize       uint64      // Total cache storage in bytes
    WriteBufferCapacity int         // Max lines in the write buffer
    MaxInflightFetch    int         // Concurrent fetches to lower memory
    MaxInflightEviction int         // Concurrent evictions to lower memory

    // Address mapping over Resources.RemotePorts, used when no
    // Resources.AddressToPortMapper is injected.
    AddressMapperType string // "single" or "interleaved"
    InterleavingSize  uint64
}
```

The number of sets is not configured directly: it is
`TotalByteSize / (WayAssociativity * 2^Log2BlockSize)`, and equals
`len(comp.State.DirectoryState.Sets)` on a built cache.

## Builder Pattern

Start from `Definition.DefaultSpec`, tweak the fields you need, and pass the
whole spec to `WithSpec`. Wiring comes from `WithSimulation` (which provides
the engine and registers the component), `WithResources` (the backing storage
plus the address-to-port mapping for lower memory), and `WithPorts` (the port
instances).

```go
spec := writeback.Definition.DefaultSpec
spec.TotalByteSize = 64 * mem.KB
spec.WayAssociativity = 4
spec.Log2BlockSize = 6 // 64-byte lines
spec.NumMSHREntry = 16

cache := writeback.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(writeback.Resources{
        Storage:             mem.NewStorage(spec.TotalByteSize),
        AddressToPortMapper: lowModuleMapper,
    }).
    WithPorts(writeback.Ports{
        Top:     messaging.NewPort("L1Cache.Top", 8, 8),
        Bottom:  messaging.NewPort("L1Cache.Bottom", 8, 8),
        Control: messaging.NewPort("L1Cache.Control", 8, 8),
    }).
    Build("L1Cache")

topPort := cache.Ports.Top
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak |
| `WithResources(Resources{...})` | Backing storage (required) and the lower-memory address mapping (`AddressToPortMapper` or `RemotePorts`) |
| `WithPorts(Ports{...})` | The port instances, each named `"<instance>.<field>"` (required) |

`Build` panics if `Resources.Storage` is nil; the cache no longer creates a
default storage. Size the storage to hold at least `Spec.TotalByteSize` bytes.

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Block size | 64 bytes (log2 = 6) |
| Way associativity | 4 |
| Banks | 1 |
| Total size | 512 KB |
| Bank latency | 10 cycles |
| Requests per cycle | 1 |
| MSHR entries | 16 |
| Write buffer capacity | 1024 |
| Max inflight fetch / eviction | 128 / 128 |
| Interleaving size | 4096 |

## Cache States

The cache operates in one of six states (the `cacheState` constants):

- **Running** — normal operation, processing read/write requests
- **Draining** — finishing in-flight transactions before pausing
- **PreFlushing** — preparing for a flush operation
- **Flushing** — evicting dirty blocks to lower memory
- **Paused** — pipeline halted (after Pause, Drain, or Flush)
- **Invalid** — uninitialized

## Ports

The system builder creates each port with `messaging.NewPort`, choosing its
buffer sizes, and passes them to `WithPorts`; `Build` binds and registers them.

- **Top**: accepts `memprotocol.ReadReq` and `memprotocol.WriteReq`, returns `memprotocol.DataReadyRsp`
  and `memprotocol.WriteDoneRsp`.
- **Bottom**: issues `memprotocol.ReadReq` (fetches) and `memprotocol.WriteReq` (evictions) to
  lower-level memory.
- **Control**: accepts `memcontrolprotocol.Req` for pause/drain/enable/reset/
  invalidate/flush operations, returns `memcontrolprotocol.Rsp`.
