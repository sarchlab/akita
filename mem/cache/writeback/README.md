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
- `Resources` — the backing `*mem.Storage` (required). Destination ports and
  mapping parameters belong to Spec, including range and fallback configuration.
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl`, `Flusher`, and `Pipeline`, run in that order every
  cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

```go
type Spec struct {
    Freq                timing.Freq // Operating frequency
    NumReqPerCycle      int         // Requests processed per cycle
    Log2BlockSize       uint64      // Cache line size = 1 << Log2BlockSize
    BankLatency         int         // Cycles per bank read/write
    DirLatency          int         // Cycles for directory lookup
    WayAssociativity    int         // Ways per set
    NumBanks            int         // Positive number of banks
    NumMSHREntry        int         // MSHR capacity
    NumSets             int         // Number of sets in the directory
    WriteBufferCapacity int         // Max lines in the write buffer
    MaxInflightFetch    int         // Concurrent fetches to lower memory
    MaxInflightEviction int         // Concurrent evictions to lower memory

    // Routing is read directly from Spec.
    Bottom mem.AddressMapper // destinations, interleaving, optional range/fallback
}
```

The directory has `NumSets` sets. Its byte capacity is
`NumSets * WayAssociativity * (1 << Log2BlockSize)`, available as `Spec.ByteSize()`.

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the
whole spec to `WithSpec`. Wiring comes from `WithSimulation` (which provides
the engine and registers the component), `WithResources` (the backing storage
only), and `component.BindPort` after Build (the port instances).

```go
spec := writeback.DefaultSpec()
spec.NumSets = 256 // 64 KiB with four ways and 64-byte lines
spec.WayAssociativity = 4
spec.Log2BlockSize = 6 // 64-byte lines
spec.NumMSHREntry = 16
spec.Bottom.Destinations = []messaging.RemotePort{"DRAM.Top"}

cache := modeling.MakeBuilder[*writeback.Comp](spec).
    WithSimulation(sim).
    WithResources(writeback.Resources{
        Storage:             mem.NewStorage(spec.ByteSize()),
    }).
    Build("L1Cache")
cache.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 8, OutgoingCapacity: 8}).
    Build())
cache.BindPort("Bottom", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 8, OutgoingCapacity: 8}).
    Build())
cache.BindPort("Control", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 8, OutgoingCapacity: 8}).
    Build())

topPort := cache.Ports.Top
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{...})` | Backing storage (required) |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

`Initialize` panics if `Resources.Storage` is nil; the cache no longer creates a
default storage. Size the storage to hold at least `Spec.ByteSize()` bytes.

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

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `memprotocol.ReadReq` and `memprotocol.WriteReq`, returns `memprotocol.DataReadyRsp`
  and `memprotocol.WriteDoneRsp`.
- **Bottom**: issues `memprotocol.ReadReq` (fetches) and `memprotocol.WriteReq` (evictions) to
  lower-level memory.
- **Control**: accepts `memcontrolprotocol.Req` for pause/drain/enable/reset/
  invalidate/flush operations, returns `memcontrolprotocol.Rsp`.

## Independent processing and buffering

`NumReqPerCycle` controls stage dispatch. `DirectoryWidth` and `BankWidth`
control pipeline admission width. Queue depths are explicit, independent fields:
`DirBufferCapacity`, `DirPostBufferCapacity`, `BankPostBufferCapacity`, `DirToBankBufferCapacity`, `WriteBufferToBankBufferCapacity`, `MSHRBufferCapacity`, `WriteBufferStageCapacity`.
Changing one field does not recalculate any other. Start from `DefaultSpec()`
and change each desired parameter explicitly. Geometry, widths and queue depths
must be positive; pipeline latencies must be nonnegative.

`BankWidth` must be at least two: the bank admission policy reserves one lane
for upward traffic. The default has request width one and directory/bank width
two; this is explicit rather than silently widening a requested width.
