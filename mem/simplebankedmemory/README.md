# simplebankedmemory — Banked Memory Controller

Package `simplebankedmemory` provides a configurable banked memory controller
for the Akita simulation framework. It models a banked memory subsystem built on
top of Akita's pipeline primitives and is a good default memory unit for most
simulations: it is not a detailed DRAM model, but provides controllable
bandwidth and latency for the common case.

## How It Works

The component exposes a single `Top` port. All clients share this port, and the
internal logic determines which bank serves each request. Each bank owns a
configurable pipeline (width, depth, per-stage latency) and a post-pipeline
buffer that holds completed items until responses can be delivered.

Requests traverse the following stages:

1. **Ingress** — Messages arriving at the `Top` port remain queued in the port's
   incoming buffer.
2. **Bank selection** — On each tick the dispatch middleware takes requests from
   the port buffer and dispatches them into the selected bank's pipeline. With
   the default `"interleaved"` selector, addresses are interleaved by a
   configurable stride (`2 ^ BankSelectorLog2InterleaveSize` bytes).
3. **Pipeline traversal** — Banks simulate execution latency by advancing all
   in-flight pipeline slots every tick.
4. **Completion** — When items exit the pipeline, reads gather their data from
   the backing `mem.Storage` while writes commit the modified bytes. Both
   generate responses once the `Top` port has space to send.

For a quick approximation, the achievable peak bandwidth is
`NumBanks × BankPipelineWidth × (1 / StageLatency) × Freq`. To keep sequential
traffic saturated, configure more banks than the pipeline latency so a stream of
requests can occupy different banks while earlier ones are still in flight.

## Key Types

- `Spec` — immutable configuration: frequency, bank geometry, pipeline shape,
  buffer sizes, capacity, bank-selection, and address-conversion fields.
- `State` — mutable runtime data: the per-bank pipelines and post-pipeline
  buffers.
- `Resources` — shared wiring; holds the backing `*mem.Storage`.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

```go
type Spec struct {
    Freq                timing.Freq // Operating frequency
    NumBanks            int         // Number of banks
    BankPipelineWidth   int         // Items entering a bank pipeline per tick
    BankPipelineDepth   int         // Pipeline stages per bank
    StageLatency        int         // Cycles per pipeline stage
    PostPipelineBufSize int         // Post-pipeline buffer depth per bank
    TopPortBufferSize   int         // Top port buffer capacity
    Capacity            uint64      // Backing-storage size when built internally
    StorageRef          string      // Storage resource name (set by Build)

    BankSelectorKind               string // "interleaved"
    BankSelectorLog2InterleaveSize uint64 // log2 of the interleave stride

    // Address conversion for interleaved multi-controller setups.
    AddrConvKind            string
    AddrInterleavingSize    uint64
    AddrTotalNumOfElements  int
    AddrCurrentElementIndex int
    AddrOffset              uint64
}
```

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the whole spec
to `WithSpec`. Wiring comes from `WithRegistrar` (which provides the engine and
registers the component) and `WithResources` (the shared backing storage). When
`WithResources` is omitted, the controller builds its own storage sized by
`Spec.Capacity`. The `Top` port is created internally by `Build`.

```go
engine := timing.NewSerialEngine()

spec := simplebankedmemory.DefaultSpec()
spec.NumBanks = 4
spec.BankPipelineWidth = 2
spec.BankPipelineDepth = 3
spec.StageLatency = 2
spec.BankSelectorLog2InterleaveSize = 6 // 64 B stride

memCtrl := simplebankedmemory.MakeBuilder().
    WithRegistrar(reg).
    WithSpec(spec).
    WithResources(simplebankedmemory.Resources{Storage: storage}).
    Build("MyMemCtrl")

topPort := memCtrl.GetPortByName("Top")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{Storage: s})` | Shared backing storage (built internally if omitted) |

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Banks | 4 |
| Bank pipeline width / depth | 1 / 1 |
| Stage latency | 10 cycles |
| Post-pipeline buffer | 1 |
| Top port buffer | 16 |
| Storage capacity | 4 GB |
| Bank selector | `"interleaved"`, 64 B stride (log2 = 6) |

## Ports

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns `mem.DataReadyRsp`
  and `mem.WriteDoneRsp`.

## Example

The package ships with a runnable example that issues sequential 64 B reads and
prints the achieved bandwidth:

```sh
go test ./mem/simplebankedmemory -run Example
```
