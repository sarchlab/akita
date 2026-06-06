# datamover — Streaming Data Mover

Package `datamover` provides a DMA-style data-mover component for the Akita
simulation framework. It copies a contiguous region of memory from a source to a
destination by issuing `mem.ReadReq`s to one side, buffering the returned data,
and issuing `mem.WriteReq`s to the other side. The mover owns no storage of its
own — it streams data between external memory controllers reachable through its
two memory-facing ports.

## How It Works

A move is driven by a single `DataMoveRequest` on the `Control` port. The
component processes one transaction at a time:

```
Control ──► DataMoveRequest ──► dataTransferMW ──► DataMoveResponse ──► Control
                                      │
              ┌───────────────────────┴────────────────────────┐
        read side (src)                                  write side (dst)
   ReadReq ──► [Inside|Outside] ──► DataReadyRsp ──► buffer ──► WriteReq ──► [Inside|Outside] ──► WriteDoneRsp
```

Each tick the `dataTransferMW` middleware:

1. **readFromSrc** — issues `mem.ReadReq`s at the source granularity, as long as
   the read window stays within the configured `BufferSize` and the requested
   region.
2. **processDataReadyFromSrc** — stores each `mem.DataReadyRsp` payload into the
   sliding buffer at its address offset.
3. **writeToDst** — once a full destination-granularity chunk is available in the
   buffer, issues a `mem.WriteReq` to the destination and advances the buffer
   offset, discarding consumed chunks.
4. **processWriteDoneFromDst** — clears the matching pending write on each
   `mem.WriteDoneRsp`.

The `ctrlParseMW` middleware parses incoming `DataMoveRequest`s (rejecting a new
one while a transaction is active) and, once every byte has been written back,
sends a `DataMoveResponse` to the original requester.

The source and destination each name one of the two sides — `"inside"` or
`"outside"` — so a move can go inside→outside, outside→inside, or same-side. The
buffer (`InsideByteGranularity` / `OutsideByteGranularity`) decouples the read
and write transfer sizes.

## Key Types

```go
type Comp = modeling.Component[Spec, State, modeling.None]

type DataMoveRequest struct {
    messaging.MsgMeta
    SrcAddress, DstAddress uint64
    ByteSize               uint64
    SrcSide, DstSide       DataMovePort // "inside" or "outside"
}

type DataMoveResponse struct {
    messaging.MsgMeta
}
```

- **Spec** — immutable config: `Freq`, `BufferSize`,
  `InsideByteGranularity`/`OutsideByteGranularity`, and the inside/outside
  address-mapper fields.
- **State** — mutable runtime: the single `CurrentTransaction` (with its pending
  read/write maps and next read/write addresses) and the sliding `Buffer`.
- **Resources** — the inside/outside `mem.AddressToPortMapper`s describing which
  remote port serves a given address on each side. They are optional; when
  omitted the flat mapper fields in `Spec` are used.

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`DefaultSpec()`); the engine and registration come from `WithRegistrar`; the
side mappers come from `WithResources`. `Build` declares the component's `Top`,
`Inside`, `Outside`, and `Control` ports; the caller builds the port instances
(choosing the buffer sizes) with `modeling.MakePortBuilder` and attaches them
with `AssignPort`.

```go
spec := datamover.DefaultSpec()
spec.BufferSize = 4096
spec.InsideByteGranularity = 64
spec.OutsideByteGranularity = 64

mover := datamover.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(datamover.Resources{
        InsideMapper:  &mem.SinglePortMapper{Port: l2Port},
        OutsideMapper: &mem.SinglePortMapper{Port: dramPort},
    }).
    Build("DMA")

for _, name := range []string{"Top", "Inside", "Outside", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(mover).
        WithSpec(modeling.PortSpec{BufSize: 16}).
        Build(name)
    mover.AssignPort(name, p)
}

ctrlPort := mover.GetPortByName("Control")
```

### Builder Methods

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()`. |
| `WithResources(r)` | The inside/outside address-to-port mappers. |

## Ports

- **Control** — accepts `DataMoveRequest`, returns `DataMoveResponse` to the
  requester once the move completes.
- **Inside** / **Outside** — the two memory-facing ports. Whichever side a move
  names as source issues `mem.ReadReq`s (receiving `mem.DataReadyRsp`); the
  destination side issues `mem.WriteReq`s (receiving `mem.WriteDoneRsp`).
