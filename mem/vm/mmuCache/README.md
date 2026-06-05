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
2. **sendReqToBottom** — A `vm.TranslationReq` is forwarded on the `Bottom` port
   to `LowModulePort`, carrying the remaining latency in its `TransLatency`
   field so the downstream provider can account for the cached levels.
3. **handleRsp** — When a `vm.TranslationRsp` returns on `Bottom`, every level is
   filled with the resolved page's segments (using LRU replacement within each
   level) and a `vm.TranslationRsp` is relayed up to `UpModulePort`.

Up to `NumReqPerCycle` lookups and responses are processed each tick. The cache
runs an `enable` / `drain` / `pause` / `flush` state machine; a flush clears all
levels.

## Key Types

- `Spec` — immutable configuration: frequency, `NumBlocks` (ways per level),
  `NumLevels`, `Log2PageSize`, `NumReqPerCycle`, `LatencyPerLevel`, and port
  buffer sizes.
- `State` — mutable runtime data: the per-level table (blocks plus `lruset.Set`),
  the current state, and inflight-flush bookkeeping.
- `Resources` — external wiring; holds `LowModulePort` (downstream provider) and
  `UpModulePort` (upstream requester).
- `Comp` — `modeling.Component[Spec, State, Resources]`.

## Builder Pattern

```go
spec := mmuCache.DefaultSpec()
spec.NumLevels = 4
spec.NumBlocks = 16
spec.LatencyPerLevel = 50

c := mmuCache.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(mmuCache.Resources{
        LowModulePort: mmuPort,
        UpModulePort:  tlbPort,
    }).
    Build("MMUCache")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak (`NumBlocks` must be > 0) |
| `WithResources(Resources{...})` | External wiring (low- and up-module remote ports) |

## Ports

- **Top**: accepts `vm.TranslationReq` from the upstream requester.
- **Bottom**: forwards `vm.TranslationReq` to the downstream provider and
  receives `vm.TranslationRsp`, which is then relayed back to `UpModulePort`.
- **Control**: accepts `mem.ControlReq` (enable / drain / pause / flush / reset)
  and returns `mem.ControlRsp` for flush and reset.
