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
2. **sendReqToBottom** — A `vmprotocol.TranslationReq` is forwarded on the `Bottom` port
   to `LowModulePort`, carrying the remaining latency in its `TransLatency`
   field so the downstream provider can account for the cached levels.
3. **handleRsp** — When a `vmprotocol.TranslationRsp` returns on `Bottom`, every level is
   filled with the resolved page's segments (using LRU replacement within each
   level) and a `vmprotocol.TranslationRsp` is relayed up to `UpModulePort`.

Up to `NumReqPerCycle` lookups and responses are processed each tick. The cache
runs an `enable` / `drain` / `pause` / `flush` state machine; a flush clears all
levels.

## Key Types

- `Spec` — immutable configuration: frequency, `NumBlocks` (ways per level),
  `NumLevels`, `Log2PageSize`, `NumReqPerCycle`, and `LatencyPerLevel`.
- `State` — mutable runtime data: the per-level table (blocks plus `lruset.Set`),
  the current state, and inflight-flush bookkeeping.
- `Resources` — external wiring; holds `LowModulePort` (downstream provider) and
  `UpModulePort` (upstream requester).
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands) and `Cache` (lookup, forwarding,
  and responses), run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Start from `Definition.DefaultSpec`, tweak the fields you need, and pass the
whole spec to `WithSpec`. Wiring comes from `WithSimulation` (which provides the
engine and registers the component), `WithResources` (the low- and up-module
remote ports), and `component.BindPort` after Build (the port instances).

```go
spec := mmuCache.Definition.DefaultSpec
spec.NumLevels = 4
spec.NumBlocks = 16
spec.LatencyPerLevel = 50

c := mmuCache.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(mmuCache.Resources{
        LowModulePort: mmuPort,
        UpModulePort:  tlbPort,
    }).
    Build("MMUCache")
c.BindPort("Top", twowaybuffered.NewPort(16, 16))
c.BindPort("Bottom", twowaybuffered.NewPort(16, 16))
c.BindPort("Control", twowaybuffered.NewPort(16, 16))
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak (`NumBlocks` must be > 0) |
| `WithResources(Resources{...})` | External wiring (low- and up-module remote ports) |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.NewPort`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `vmprotocol.TranslationReq` from the upstream requester.
- **Bottom**: forwards `vmprotocol.TranslationReq` to the downstream provider and
  receives `vmprotocol.TranslationRsp`, which is then relayed back to `UpModulePort`.
- **Control**: accepts `memcontrolprotocol.Req` (enable / drain / pause / flush / reset)
  and returns `memcontrolprotocol.Rsp` for flush and reset.
