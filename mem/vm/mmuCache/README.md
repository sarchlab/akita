# mmuCache — MMU Translation Cache

Package `mmuCache` provides a translation cache for the Akita simulation
framework. It sits in the translation path of the virtual-memory subsystem —
between an upstream requester (such as a TLB) and a downstream translation
provider (such as the MMU) — and models a multi-level page-walk cache that
shortens the effective page-walk latency on partial hits.

## How It Works

The cache holds `NumLevels` levels, each a small set of blocks keyed by
`(PID, segment)`, where a segment is one slice of the virtual page number. The
virtual page number (`VAddr >> Log2PageSize`) is split into `NumLevels` equal
segments.

On a request arriving from the `Top` port:

1. **walkCacheLevels** — Starting from the highest level, the cache probes each
   level for the corresponding segment. Each level that hits subtracts
   `LatencyPerLevel` from the total walk latency; the walk stops at the first
   miss.
2. **sendReqToBottom** — A `addrtransprotocol.Req` is forwarded on the `Bottom` port
   to `Spec.LowModule`, carrying the remaining latency in its `TransLatency`
   field so the downstream provider can account for the cached levels.
3. **handleRsp** — When a `addrtransprotocol.Rsp` returns on `Bottom`, every level is
   filled with the resolved page's segments (using LRU replacement within each
   level) and a `addrtransprotocol.Rsp` is relayed up to `Spec.UpModule`.

Up to `NumReqPerCycle` lookups and responses are processed each tick. The cache
runs an `enable` / `drain` / `pause` / `flush` state machine; a flush clears all
levels.

## Key Types

- `Spec` — immutable configuration: frequency, `NumBlocks` (ways per level),
  `NumLevels`, `Log2PageSize`, `NumReqPerCycle`, `LatencyPerLevel`, and the
  peer addresses `LowModule` (downstream) and `UpModule` (upstream).
- `State` — mutable runtime data: the per-level table (blocks plus `lruset.Set`),
  the current state, and inflight-flush bookkeeping.
- `Resources` — empty; peer addresses belong to `Spec`.
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands) and `Cache` (lookup, forwarding,
  and responses), run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the
whole spec to `WithSpec`. Wiring comes from `WithSimulation` (which provides the
engine and registers the component) and `component.BindPort` after Build
(the port instances). Set peer addresses in `Spec.LowModule` and `Spec.UpModule`.

```go
spec := mmuCache.DefaultSpec()
spec.NumLevels = 4
spec.NumBlocks = 16
spec.LatencyPerLevel = 50
spec.LowModule = mmuPort
spec.UpModule = tlbPort

c := modeling.MakeBuilder[*mmuCache.Comp](spec).
    WithSimulation(sim).
    Build("MMUCache")
c.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
c.BindPort("Bottom", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
c.BindPort("Control", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak (`NumBlocks` must be > 0) |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `addrtransprotocol.Req` from the upstream requester.
- **Bottom**: forwards `addrtransprotocol.Req` to the downstream provider and
  receives `addrtransprotocol.Rsp`, which is then relayed back to `Spec.UpModule`.
- **Control**: accepts `memcontrolprotocol.Req` (enable / drain / pause / flush / reset)
  and returns `memcontrolprotocol.Rsp` for flush and reset.
