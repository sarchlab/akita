# gmmu — GPU Memory Management Unit

Package `gmmu` provides a GPU memory management unit for the Akita simulation
framework. It is the GPU-side counterpart of the `mmu` package within the
virtual-memory subsystem. The GMMU walks the page table for translations whose
page is local to its device, and forwards translations for remote pages down to
a lower-level translation provider (typically the CPU-side MMU).

## How It Works

The GMMU is configured with a `DeviceID` and is driven by two middlewares.

### walkMW — top→page-table path

1. **parseFromTop** — Accepts a `vm.TranslationReq` from the `Top` port (up to
   `MaxRequestsInFlight` in flight) and starts a walk with a `Latency`-cycle
   countdown.
2. **walkPageTable** — Each tick decrements every walk. On completion it looks up
   the page in the shared `vm.PageTable`:
   - If `page.DeviceID == DeviceID` (local), it finalizes the walk and returns a
     `vm.TranslationRsp` on `Top`.
   - Otherwise it forwards a `vm.TranslationReq` on the `Bottom` port to the
     configured `LowModule`, remembering the transaction by request ID.

### respondMW — bottom→top path

Reads `vm.TranslationRsp` messages arriving on `Bottom`, matches them to the
remembered remote request, and relays a `vm.TranslationRsp` back up on `Top`.

## Key Types

- `Spec` — immutable configuration: frequency, `DeviceID`, `Log2PageSize`, walk
  `Latency`, `MaxRequestsInFlight`, and the `LowModule` remote port.
- `State` — mutable runtime data: in-flight walks, the map of remote memory
  requests awaiting responses, and per-device page-access tracking.
- `Resources` — shared wiring; holds the `vm.PageTable`. If none is supplied the
  builder constructs one sized by `Log2PageSize`.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the whole spec
to `WithSpec`. Wiring comes from `WithRegistrar` (which provides the engine and
registers the component) and `WithResources` (the shared page table). When
`WithResources` is omitted, the GMMU builds its own page table sized by
`Spec.Log2PageSize`. `Build` declares the `Top`, `Bottom`, and `Control` ports
but does not create their instances. Build each port with
`modeling.MakePortBuilder` (which registers the port with the simulation) and
attach it with `AssignPort`, choosing the buffer size.

```go
spec := gmmu.DefaultSpec()
spec.DeviceID = 1
spec.LowModule = mmuPort

g := gmmu.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(gmmu.Resources{PageTable: pageTable}).
    Build("GMMU")

for _, name := range []string{"Top", "Bottom", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(g).
        WithSpec(modeling.PortSpec{BufSize: 16}).
        Build(name)
    g.AssignPort(name, p)
}
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{PageTable: pt})` | Shared page table (built internally if omitted) |

## Ports

- **Top**: accepts `vm.TranslationReq`, returns `vm.TranslationRsp`.
- **Bottom**: forwards `vm.TranslationReq` for remote pages, receives
  `vm.TranslationRsp`.
- **Control**: accepts `mem.ControlReq` (enable / pause / drain / reset),
  returns `mem.ControlRsp`.
