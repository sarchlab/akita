# tlb — Translation Lookaside Buffer

Package `tlb` provides a translation lookaside buffer for the Akita simulation
framework. It is part of the virtual-memory subsystem: it caches recent
virtual-to-physical address translations so that repeated translations hit
locally, and forwards misses down to the MMU (or another translation provider).

## How It Works

The TLB is a set-associative cache of `vm.Page` entries keyed by `(PID, VAddr)`.
Translation requests arrive on the `Top` port as `addrtransprotocol.Req` messages and
flow through a fixed-latency pipeline before lookup:

1. **Insert / pipeline** — Incoming requests are accepted into a
   `queueing.FastPipeline` that imposes `Latency` cycles. Up to `NumReqPerCycle`
   requests move per tick. A latency of zero bypasses the pipeline. The TLB
   sleeps while requests only travel and ticks again when one is ready.
2. **Lookup** — On exit from the pipeline the request is looked up in the set
   selected by `vAddr / 2^Log2PageSize % NumSets`.
   - **Hit** (entry present and `Valid`): a `addrtransprotocol.Rsp` carrying the
     `vm.Page` is sent back on `Top`, and the matched way is marked
     most-recently-used.
   - **MSHR hit**: an outstanding miss already covers this
     `(PID, VAddr, NoMigrate)`, so the request is attached to the existing MSHR
     entry.
   - **Miss**: an MSHR entry is allocated and a `addrtransprotocol.Req`, with the
     request's `NoMigrate`, is forwarded on the `Bottom` port to the
     translation provider resolved by the Spec routing value.
3. **Fill / respond** — A response returning on `Bottom` is matched to its MSHR
   entry by the ID of the forwarded request. It reuses the way of the page if
   the page is already valid in the set (filled by the other of a `NoMigrate`
   and a normal miss), and otherwise evicts an LRU way, also for a page
   invalidated in place. It installs the page and replays every request queued in that entry back to `Top`. A
   fault response (`addrtransprotocol.Rsp.Fault`) is forwarded to the queued
   requests but not filled.

A `NoMigrate` miss and a normal miss of the same page use separate MSHR
entries, so neither gets the other's result. Hits ignore `NoMigrate`: a page
filled by a `NoMigrate` miss, which may be placed on another device, answers
later normal requests too. A TLB that serves `NoMigrate` requesters, such as a
data mover's, is therefore kept apart from the TLBs of compute units.

Replacement order within each set is tracked by `lruset.Set`; in-flight misses
are tracked in `State.MSHREntries`.

## Key Types

- `Spec` — immutable configuration (frequency, geometry, latency, MSHR size).
- `State` — mutable runtime data: per-set blocks and LRU order, MSHR entries, the
  request pipeline, and flush bookkeeping. The TLB runs a small state machine:
  `enable`, `drain`, `pause`, and `flush`.
- `Resources` — empty. Routing uses `mem.AddressMapper` values in Spec (Bottom).
  Middleware calls their `Find` methods directly; Resources has no routing override.
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands) and `TLB` (lookup, misses, and
  responses), run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

```go
spec := tlb.DefaultSpec()
spec.NumSets = 64
spec.NumWays = 4
spec.NumMSHREntries = 8
spec.Bottom.Destinations = []messaging.RemotePort{"MMU.Top"}

t := modeling.MakeBuilder[*tlb.Comp](spec).
    WithSimulation(sim).
    Build("L2TLB")
t.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
t.BindPort("Bottom", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
t.BindPort("Control", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
    Build())
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{})` | Optional empty Resources |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `addrtransprotocol.Req`, returns `addrtransprotocol.Rsp`.
- **Bottom**: forwards `addrtransprotocol.Req` on a miss, receives `addrtransprotocol.Rsp`.
- **Control**: accepts `memcontrolprotocol.Req` (enable / drain / pause / flush / reset)
  and returns `memcontrolprotocol.Rsp` for flush and reset.

## Processing width and internal buffering

`NumReqPerCycle` sets lookup/pipeline width and the calculated output queue
capacity; the default is four and the width must be positive. The output queue
has no public capacity override. `NumSets` and `NumWays` specify TLB geometry;
`NumMSHREntries` independently sets the capacity for outstanding translation
misses.
