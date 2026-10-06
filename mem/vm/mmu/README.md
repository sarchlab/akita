# mmu — Memory Management Unit

Package `mmu` provides a memory management unit for the Akita simulation
framework. It is the CPU-side terminus of the virtual-memory subsystem:
it performs page-table walks to resolve `vmprotocol.TranslationReq` messages.

## How It Works

The MMU is driven by two middlewares, run in this order every cycle.

### Ctrl — control commands

Handles the control commands on the `Control` port: Pause, Enable, Drain, and
Reset. Drain is acknowledged once no page-table walks are in progress.
Invalidate and Flush are answered as unsupported.

### Translation — page-table walks

1. **parseFromTop** — Accepts a `vmprotocol.TranslationReq` from the `Top` port (up to
   `MaxRequestsInFlight` may be walking at once) and starts a walk with a
   countdown of `Latency` cycles.
2. **walkPageTable** — Each tick decrements every walk's countdown. When a walk
   completes it looks up the page in the shared `vm.PageTable`:
   - If the page is missing and `AutoPageAllocation` is set, a new page is
     created and inserted (otherwise it panics).
   - A `vmprotocol.TranslationRsp` is then sent back on `Top`.

## Key Types

- `Spec` — immutable configuration: frequency, walk `Latency`,
  `MaxRequestsInFlight`, `AutoPageAllocation`, and `Log2PageSize`. Port buffer
  sizes are not part of the spec; the system builder chooses them when it
  creates the port instances.
- `State` — mutable runtime data: the control state, the in-flight walks, and
  the next physical page to allocate.
- `Resources` — shared wiring; holds the `vm.PageTable`. It is required: `Build`
  panics if `PageTable` is nil, or if its page size does not match
  `Log2PageSize`. (The MMU no longer builds a default page table; build one with
  `vm.MakePageTableBuilder()` or `vm.NewPageTable(log2PageSize)`.)
- `Ports` — the `Top` and `Control` ports.
- `Middlewares` — `Ctrl` (control commands) and `Translation` (page-table
  walks), run in that order every cycle.
- `Comp` — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

```go
spec := mmu.Definition.DefaultSpec
spec.Latency = 100
spec.AutoPageAllocation = true

m := mmu.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(mmu.Resources{PageTable: pageTable}).
    WithPorts(mmu.Ports{
        Top:     twowaybuffered.NewPort("MMU.Top", 16, 16),
        Control: twowaybuffered.NewPort("MMU.Control", 4, 4),
    }).
    Build("MMU")
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak |
| `WithResources(Resources{PageTable: pt})` | Shared page table (required) |
| `WithPorts(Ports{...})` | The port instances, each named `"<instance>.<field>"` (required) |

## Ports

The system builder creates each port with `twowaybuffered.NewPort`, choosing its
buffer sizes, and passes them to `WithPorts`; `Build` binds and registers them.

- **Top**: accepts `vmprotocol.TranslationReq`, returns `vmprotocol.TranslationRsp`.
- **Control**: accepts `memcontrolprotocol.Req` (Pause, Drain, Enable, Reset), returns
  `memcontrolprotocol.Rsp`.
