# mmu — Memory Management Unit

Package `mmu` provides a memory management unit for the Akita simulation
framework. It is the CPU-side terminus of the virtual-memory subsystem:
it performs page-table walks to resolve `vm.TranslationReq` messages and, for
unified-memory pages, coordinates on-demand page migration with the driver.

## How It Works

The MMU is driven by two middlewares that run each tick.

### translationMW — page-table walks

1. **parseFromTop** — Accepts a `vm.TranslationReq` from the `Top` port (up to
   `MaxRequestsInFlight` may be walking at once) and starts a walk with a
   countdown of `Latency` cycles.
2. **walkPageTable** — Each tick decrements every walk's countdown. When a walk
   completes it looks up the page in the shared `vm.PageTable`:
   - If the page is missing and `AutoPageAllocation` is set, a new page is
     created and inserted (otherwise it panics).
   - With `RemoteFaults`, if the request may fault (`CanFault`), the page is on
     another device than the requester, and the page is not pinned, the walk
     faults: a `vm.TranslationRsp` with `Fault` set is sent back on `Top` at
     once, and a `vm.PageFault` is sent on the `Fault` port to
     `FaultHandler`. The walk is not held while the fault is serviced; the
     requester waits and translates again after the fault handler releases it.
   - Otherwise a `vm.TranslationRsp` is sent back on `Top`.

## Key Types

- `Spec` — immutable configuration: frequency, walk `Latency`,
  `MaxRequestsInFlight`, `AutoPageAllocation`, `Log2PageSize`, `RemoteFaults`,
  and `FaultHandler`. Port buffer
  sizes are no longer part of the spec; they are chosen by the caller when the
  port instances are built.
- `State` — mutable runtime data: in-flight walks, the next physical page to
  allocate, and the number of faulted walks.
- `Resources` — shared wiring; holds the `vm.PageTable`. If none is supplied the
  builder constructs one sized by `Log2PageSize`.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

## Builder Pattern

`Build` declares the component's `Top` and `Control` ports but does not create
the port instances. The caller builds each port with `modeling.MakePortBuilder`
(choosing the buffer size) and attaches it with `AssignPort`.

```go
spec := mmu.DefaultSpec()
spec.Latency = 100
spec.AutoPageAllocation = true

m := mmu.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(mmu.Resources{PageTable: pageTable}).
    Build("MMU")

for _, name := range []string{"Top", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(m).
        WithSpec(modeling.PortSpec{BufSize: 16}).
        Build(name)
    m.AssignPort(name, p)
}
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{PageTable: pt})` | Shared page table (built internally if omitted) |

## Ports

`Build` declares these ports; the caller assigns the instances after `Build`.

- **Top**: accepts `vm.TranslationReq`, returns `vm.TranslationRsp`.
- **Fault** (only with `RemoteFaults`): sends `vm.PageFault` to `FaultHandler`.
- **Control**: accepts `mem.ControlReq` (Pause, Drain, Enable, Reset), returns
  `mem.ControlRsp`.
