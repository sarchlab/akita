# writethrough — Unified GPU Cache

Package `writethrough` provides a set-associative, banked, pipelined cache
component for the Akita simulation framework. Despite the name, it is a unified
GPU cache that supports three write policies selected by a single spec field:
`write-around` (default), `write-evict`, and `write-through`. It sits between an
upper-level requester (its `Top` port) and lower-level memory (its `Bottom`
port), serving hits locally and forwarding misses downstream.

## How It Works

Each incoming `memprotocol.ReadReq`/`memprotocol.WriteReq` becomes a `transactionState` that
flows through a multi-stage pipeline, driven once per tick by `pipelineMW`:

```
Top ──► intake ──► directory(+MSHR) ──► bank(s) ──► respond ──► Top
                          │                  ▲
                          └──► Bottom ──► bottomParser
```

1. **intake** — Accepts requests from `Top` (up to `NumReqPerCycle`/tick,
   capped by `MaxNumConcurrentTrans`), creates a transaction, and pushes it into
   the directory buffer.
2. **directory** — After a `DirLatency`-cycle pipeline, looks the line up in the
   shared `cache.DirectoryState` and `cache.MSHRState`. Read hits go to a bank;
   read misses allocate an MSHR entry and fetch from `Bottom` (coalescing
   duplicate misses). Writes dispatch to the active write policy.
3. **bank** — One or more banks apply a `BankLatency`-cycle pipeline, then read
   or write the backing `mem.Storage` and unlock the directory block.
4. **bottomParser** — Parses `memprotocol.DataReadyRsp`/`memprotocol.WriteDoneRsp` from `Bottom`,
   fills MSHR entries, and merges coalesced data.
5. **respond** — Returns `memprotocol.DataReadyRsp`/`memprotocol.WriteDoneRsp` to `Top` once a
   transaction's bank, fetch, and lower-memory dependencies are all satisfied.

A separate control middleware, `Ctrl`, runs before the pipeline every cycle and
handles the commands on the `Control` port (see [Ports](#ports)).

## Write Policies

| `WritePolicyType` | On a write hit | On a write miss |
|---|---|---|
| `write-around` | Write line locally and write through to `Bottom`. | Write straight through to `Bottom`; line is not allocated. |
| `write-evict` | Write through to `Bottom` and invalidate the local line. | Write through to `Bottom`; line is not allocated. |
| `write-through` | Write line locally and write through to `Bottom`. | Allocate a victim line; partial writes fetch-and-merge through `Bottom`, full-line writes install directly. |

## Key Types

- **Spec** — immutable config: `Freq`, `Log2BlockSize`, `NumWays`,
  `NumSets`, `NumBanks`, `NumMSHREntries`, `NumReqPerCycle`,
  `MaxNumConcurrentTrans`, `BankLatency`, `DirLatency`, `WritePolicyType`, and
  the address-mapper fields. `NumSets` specifies the directory geometry;
  `Spec.ByteSize()` reports its byte capacity. An empty
  `WritePolicyType` means `write-around`.
- **State** — mutable runtime: `cache.DirectoryState`, `cache.MSHRState`, the
  flat `Transactions` list, the directory/bank `queueing.Buffer`/`Pipeline`
  stages, the pause and drain flags, and the in-progress control command.
- **Resources** — the backing `*mem.Storage` (required; at least `Spec.ByteSize()`
  bytes). Destinations and mapping choices are immutable Spec configuration:
  `Bottom mem.AddressMapper` (destinations, interleaving size, and optional
  range/fallback). Middleware routes directly through this Spec value.
- **Ports** — the `Top`, `Bottom`, and `Control` ports.
- **Middlewares** — `Ctrl` (control commands) and `Pipeline` (the data
  pipeline), run in that order every cycle.
- **Comp** — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`DefaultSpec()`); the engine and registration come from
`WithSimulation`; storage comes from
`WithResources`; the port instances come from `component.BindPort` after Build.

```go
spec := writethrough.DefaultSpec()
spec.WritePolicyType = "write-through"
spec.NumSets = 512 // 256 KiB with eight ways and 64-byte lines
spec.NumWays = 8
spec.Bottom.Destinations = []messaging.RemotePort{"DRAM.Top"}

cache := modeling.MakeBuilder[*writethrough.Comp](spec).
    WithSimulation(sim).
    WithResources(writethrough.Resources{
        Storage:       mem.NewStorage(spec.ByteSize()),
    }).
    Build("L2Cache")
cache.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
cache.BindPort("Bottom", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
cache.BindPort("Control", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())

topPort := cache.Ports.Top
```

### Builder Methods

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()`. |
| `WithResources(r)` | Backing storage (required). |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top** — accepts `memprotocol.ReadReq` and `memprotocol.WriteReq`, returns
  `memprotocol.DataReadyRsp` and `memprotocol.WriteDoneRsp`.
- **Bottom** — sends `memprotocol.ReadReq`/`memprotocol.WriteReq` to lower memory and receives
  `memprotocol.DataReadyRsp`/`memprotocol.WriteDoneRsp`.
- **Control** — accepts `memcontrolprotocol.Req` (Pause, Drain, Enable, Reset,
  and, once paused, Invalidate and Flush), returns `memcontrolprotocol.Rsp`.

## Processing widths and internal buffering

`NumReqPerCycle` controls stage dispatch. `DirectoryWidth` and `BankWidth`
control pipeline admission width, in entries per cycle. Internal request queues
use `NumReqPerCycle`; the directory output queue uses `DirectoryWidth`, and each
bank output queue uses `BankWidth`. Queue sizes are calculated at initialization
and have no public override. These sizes hold one processing batch and may
stall under contention. Geometry and widths must be positive; pipeline latencies
must be nonnegative.

`NumSets` and `NumWays` define geometry. `NumMSHREntries` remains the public
capacity for outstanding cache-line misses; it is separate from the internal
MSHR work queue.
