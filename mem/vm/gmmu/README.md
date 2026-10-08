# gmmu — GPU Memory Management Unit

Package `gmmu` provides a GPU memory management unit for the Akita simulation
framework. It is the GPU-side counterpart of the `mmu` package within the
virtual-memory subsystem. The GMMU walks the page table for translations whose
page is local to its device, and forwards translations for remote pages down to
a lower-level translation provider (typically the CPU-side MMU).

## How It Works

The GMMU is configured with a `DeviceID` and is driven by two middlewares
(after `ctrlMiddleware`, which handles control commands).

### walkMW — top→page-table path

1. **parseFromTop** — Accepts a `vmprotocol.TranslationReq` from the `Top` port (up to
   `MaxRequestsInFlight` in flight) and starts a walk with a `Latency`-cycle
   countdown.
2. **walkPageTable** — Each tick decrements every walk. On completion it looks up
   the page in the shared `vm.PageTable`:
   - If `page.DeviceID == DeviceID` (local), it finalizes the walk and returns a
     `vmprotocol.TranslationRsp` on `Top`.
   - Otherwise it forwards a `vmprotocol.TranslationReq` on the `Bottom` port to the
     configured `LowModule`, remembering the transaction by request ID.

### respondMW — bottom→top path

Reads `vmprotocol.TranslationRsp` messages arriving on `Bottom`, matches them to the
remembered remote request, and relays a `vmprotocol.TranslationRsp` back up on `Top`.

## Key Types

- `Spec` — immutable configuration: frequency, `DeviceID`, `Log2PageSize`, walk
  `Latency`, `MaxRequestsInFlight`, and the `LowModule` remote port.
- `State` — mutable runtime data: in-flight walks, the map of remote memory
  requests awaiting responses, and per-device page-access tracking.
- `Resources` — shared wiring; holds the `vm.PageTable`, which is required.
- `Ports` — the `Top`, `Bottom`, and `Control` ports.
- `Middlewares` — `Ctrl` (control commands), `Walk` (walkMW), and `Respond`
  (respondMW), run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Start from `Definition.DefaultSpec`, tweak the fields you need, and pass the whole spec
to `WithSpec`. Wiring comes from `WithSimulation` (which provides the engine and
registers the component), `WithResources` (the page table, required), and
`component.BindPort` after Build (the port instances). The GMMU does not build a page table of its
own: the system builder creates one, usually with page size
`2^Spec.Log2PageSize`, and may share it with other components. `Build` panics
if `Resources.PageTable` is nil.

```go
spec := gmmu.Definition.DefaultSpec
spec.DeviceID = 1
spec.LowModule = mmuPort

pageTable := vm.MakePageTableBuilder().
    WithLog2PageSize(spec.Log2PageSize).
    WithSimulation(sim).
    Build("GMMU.PageTable")

g := gmmu.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(gmmu.Resources{PageTable: pageTable}).
    Build("GMMU")
g.BindPort("Top", twowaybuffered.NewPort(16, 16))
g.BindPort("Bottom", twowaybuffered.NewPort(16, 16))
g.BindPort("Control", twowaybuffered.NewPort(16, 16))
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak |
| `WithResources(Resources{PageTable: pt})` | Shared page table (required) |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.NewPort`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top**: accepts `vmprotocol.TranslationReq`, returns `vmprotocol.TranslationRsp`.
- **Bottom**: forwards `vmprotocol.TranslationReq` for remote pages, receives
  `vmprotocol.TranslationRsp`.
- **Control**: accepts `memcontrolprotocol.Req` (enable / pause / drain / reset),
  returns `memcontrolprotocol.Rsp`.
