# mem

Package `mem` and its sub-packages define the memory subsystem for Akita
simulations, including the memory protocol, storage abstraction, address
mapping utilities, and implementations of caches, DRAM, and virtual memory.

## Protocol

The memory protocol (package `mem/memprotocol`) defines request/response
messages between memory components (caches, memory controllers, compute
units), with `requester`/`responder` roles that ports bind to.

### Access Messages

| Type | Direction | Fields |
|------|-----------|--------|
| `ReadReq` | Request | `Address`, `AccessByteSize`, `PID` |
| `WriteReq` | Request | `Address`, `Data []byte`, `DirtyMask []bool`, `PID` |
| `DataReadyRsp` | Response | `Data []byte` |
| `WriteDoneRsp` | Response | *(empty — acknowledgment only)* |

All protocol payloads travel inside `messaging.Msg`, whose routing fields are
`Src`, `Dst`, `ID`, and `RspTo`. Payload types contain only protocol fields.

Consumers switch on concrete `ReadReq` and `WriteReq` payloads and read their
`Address` and `PID` fields. There is no shared request or response interface.

### Control Messages

`memcontrolprotocol.Req` and `memcontrolprotocol.Rsp` (package `mem/memcontrolprotocol`) carry the uniform
control protocol used by every memory agent: Pause, Drain, Enable, Reset,
Invalidate, Flush. Each component exposes a `Control` port that carries
these messages and only these messages.

See [`CONTROL_PROTOCOL.md`](CONTROL_PROTOCOL.md) for verb definitions,
response timing, the support matrix, and how to implement and test the
protocol in a new component. The reusable state enum and conformance
harness live in `mem/memcontrolprotocol/` (see
[`memcontrolprotocol/README.md`](memcontrolprotocol/README.md)).

## Storage

`Storage` is a sparse, page-based byte store that models physical memory:

```go
storage := mem.NewStorage(4 * mem.GB)

// Write data
storage.Write(0x1000, []byte{0xDE, 0xAD})

// Read data
data := storage.Read(0x1000, 2)
```

Empty, overflowing, or out-of-capacity ranges panic before allocation or mutation.

A `Storage` can be registered with the simulation as shared state via
`NewStorageResource(name, storage)`, making its contents reachable by name
through the global state manager.

Capacity constants: `KB`, `MB`, `GB`, `TB`.

## Address Mapping

### InterleavingConverter

Converts between external (global) and internal (per-element) addresses for
interleaved address spaces:

```go
conv := mem.InterleavingConverter{
    InterleavingSize:    4096,
    TotalNumOfElements:  8,
    CurrentElementIndex: 2,
    Offset:              0,
}
internal := conv.ConvertExternalToInternal(externalAddr)
```

### AddressMapper

`mem.AddressMapper` selects the `messaging.RemotePort` responsible for an
address. Store it in the component's Spec under the routed port name, such as
`Bottom`, `Translation`, `Inside`, or `Outside`. The `AddressToPortMapper`
interface remains available.

```go
mapper := mem.AddressMapper{
    Destinations:     []messaging.RemotePort{"DRAM0.Top", "DRAM1.Top"},
    InterleavingSize: 256,
    RangeLow:         1024, // inclusive
    RangeHigh:        4096, // exclusive; zero disables filtering
    Fallback:         "OtherMemory.Top",
}
```

Component initialization requires at least one nonempty destination and a positive
`InterleavingSize` when there are multiple destinations. A single destination
needs no interleaving size. Interleaving uses the absolute address and wraps
across the destinations without translating the address.

An optional `[RangeLow, RangeHigh)` range requires `RangeHigh > RangeLow` and a
nonempty `Fallback`; addresses outside the range go to that fallback. Setting
`RangeHigh` to zero disables filtering and requires no fallback, including for
an unbounded single destination.

See the [migration guide](../doc/tutorial/migration.md#construction-binding-and-initialization)
for the removed concrete mapper types and checkpoint compatibility changes.

## Sub-Packages

| Package | Description |
|---------|-------------|
| `mem/cache` | Cache implementations (L1, L2, etc.) |
| `mem/dram` | DRAM controller models |
| `mem/vm` | Virtual memory / page table management |
| `mem/idealmemcontroller` | Ideal (zero-latency) memory controller |
| `mem/mshr` | Miss Status Holding Registers |
| `mem/datamover` | Data movement between memory components |
| `mem/simplebankedmemory` | Simple banked memory model |
