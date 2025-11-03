# SimpleBankedMemory

`SimpleBankedMemory` is a configurable memory controller that models a
banked memory subsystem built on top of Akita’s pipeline primitives. It is
recommended to be the default memory unit to be used in most of the
simulations. It is not a super detailed DRAM model, but can provide good
bandwidth and latency control for most of the cases.


## Component Overview

The component exposes a single top port. All clients share this port and the
internal logic determines which bank will eventually serve a request. Each
bank owns:

- A configurable pipeline (width, depth, per-stage latency)
- A post-pipeline buffer that holds completed items until responses can be
  delivered

Requests traverse the following stages:

1. **Ingress:** Messages arriving at the top port remain queued in the port’s
   incoming buffer.
2. **Bank selection:** On every tick up to one request per bank is taken from
   the port buffer and dispatched into the bank’s pipeline. The default bank
   selector distributes addresses by interleaving (configurable stride).
3. **Pipeline traversal:** Banks simulate execution latency by advancing all
   in-flight pipeline slots every tick.
4. **Completion:** When items exit the pipeline, reads gather their data from
   the backing `mem.Storage` while writes commit the modified bytes. Both
   generate responses once the top port has space to send.


## Key Properties

- **Banking policy:** Configurable via the builder. By default addresses are
  interleaved using a 64 B stride (log₂ value is adjustable).
- **Slide-in pipelines:** Each bank is a first-class pipeline from the
  `pipelining` package, allowing you to control width, depth, and per-stage
  latency.
- **Bandwidth modeling:** For a quick approximation, the achievable peak
  bandwidth is

  ```
  numBanks × pipelineWidth × (1 / stageLatency) × frequency
  ```

- **Storage semantics:** Reads are consistent with writes that finish earlier
  in the same cycle. Writes apply at completion; reads that finish later in
  the cycle observe the updated data.


## Building a Memory Instance

```go
engine := sim.NewSerialEngine()
memCtrl := simplebankedmemory.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithNumBanks(4).
    WithBankPipelineWidth(2).
    WithBankPipelineDepth(3).
    WithStageLatency(2).
    WithTopPortBufferSize(16).
    WithPostPipelineBufferSize(32).
    WithLog2InterleaveSize(6). // 64 B stride
    Build("MyMemCtrl")
```

The controller exposes a single port named `"Top"` that should be connected to
upstream components (e.g., caches or agents) via an Akita connection.


## Example

The package ships with a runnable example that issues 100,000 sequential 64 B
reads and prints the achieved bandwidth. You can execute it with:

```sh
go test ./mem/simplebankedmemory -run Example
```

The example demonstrates how to create an agent, wire it to the controller,
and drive the simulation loop until all responses arrive.


## Extensibility

- Implement your own `bankSelector` by supplying `WithBankSelector` on the
  builder. A selector maps an address to a bank index.
- Replace the default storage with `WithStorage` to share memory across
  controllers or to preload data.
- Because the component is built on top of Akita’s middleware system, you can
  attach additional middlewares for tracing or statistics gathering.


## Tests

Unit tests live within the package and cover:

- Basic read latency and write commit behavior
- The bandwidth example as a documentation test

Run them with:

```sh
go test ./mem/simplebankedmemory
```
