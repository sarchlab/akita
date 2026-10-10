# datamover — Streaming Data Mover

Package `datamover` provides a DMA-style data-mover component for the Akita
simulation framework. It copies a contiguous region of memory from a source to a
destination by issuing `memprotocol.ReadReq`s to one side, buffering the returned data,
and issuing `memprotocol.WriteReq`s to the other side. The mover owns no storage of its
own — it streams data between external memory controllers reachable through its
two memory-facing ports.

## How It Works

A move is driven by a single `DataMoveReq` on the `Top` port. The
component processes one transaction at a time:

```
Top ──► DataMoveReq ──► dataTransferMW ──► DataMoveRsp ──► Top
                                      │
              ┌───────────────────────┴────────────────────────┐
        read side (src)                                  write side (dst)
   ReadReq ──► [Inside|Outside] ──► DataReadyRsp ──► buffer ──► WriteReq ──► [Inside|Outside] ──► WriteDoneRsp
```

Each tick the `dataTransferMW` middleware:

1. **readFromSrc** — issues `memprotocol.ReadReq`s at the source granularity, as long as
   the read window stays within the configured `BufferSize` and the requested
   region.
2. **processDataReadyFromSrc** — stores each `memprotocol.DataReadyRsp` payload into the
   sliding buffer at its address offset.
3. **writeToDst** — once a full destination-granularity chunk is available in the
   buffer, issues a `memprotocol.WriteReq` to the destination and advances the buffer
   offset, discarding consumed chunks.
4. **processWriteDoneFromDst** — clears the matching pending write on each
   `memprotocol.WriteDoneRsp`.

The `ctrlParseMW` middleware parses incoming `DataMoveReq`s (rejecting a new
one while a transaction is active) and, once every byte has been written back,
sends a `DataMoveRsp` to the original requester.

The source and destination each name one of the two sides — `"inside"` or
`"outside"` — so a move can go inside→outside, outside→inside, or same-side. The
buffer (`InsideByteGranularity` / `OutsideByteGranularity`) decouples the read
and write transfer sizes.

## Key Types

```go
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]

type DataMoveReq struct {
    SrcAddress, DstAddress uint64
    ByteSize               uint64
    SrcSide, DstSide       DataMovePort // "inside" or "outside"
}

type DataMoveRsp struct {
}
```

- **Spec** — immutable config: `Freq`, `BufferSize`, and
  `InsideByteGranularity`/`OutsideByteGranularity`.
- **State** — mutable runtime: the single `CurrentTransaction` (with its pending
  read/write maps and next read/write addresses) and the sliding `Buffer`.
- **Resources** — the inside/outside `mem.AddressToPortMapper`s describing which
  remote port serves a given address on each side. They are not checkpointed;
  the setup that rebuilds the data mover supplies them. A move panics if the
  mapper for a side it touches is nil.
- **Ports** — the `Top`, `Inside`, `Outside`, and `Control` ports.
- **Middlewares** — `Ctrl` (control commands), `CtrlParse` (admits moves and
  completes finished ones), and `DataTransfer` (reads and writes), run in that
  order every cycle.
- **Comp** — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`Definition.DefaultSpec`); the engine and registration come from
`WithSimulation`; the side mappers come from `WithResources`; the port
instances come from `component.BindPort` after Build.

```go
spec := datamover.Definition.DefaultSpec
spec.BufferSize = 4096
spec.InsideByteGranularity = 64
spec.OutsideByteGranularity = 64

mover := datamover.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(datamover.Resources{
        InsideMapper:  &mem.SinglePortMapper{Port: l2Port},
        OutsideMapper: &mem.SinglePortMapper{Port: dramPort},
    }).
    Build("DMA")
mover.BindPort("Top", twowaybuffered.NewPort(16, 16))
mover.BindPort("Inside", twowaybuffered.NewPort(16, 16))
mover.BindPort("Outside", twowaybuffered.NewPort(16, 16))
mover.BindPort("Control", twowaybuffered.NewPort(16, 16))

ctrlPort := mover.Ports.Control
```

### Builder Methods

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec`. |
| `WithResources(r)` | The inside/outside address-to-port mappers. |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.NewPort`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top** — accepts `DataMoveReq`, returns `DataMoveRsp` to the
  requester once the move completes.
- **Inside** / **Outside** — the two memory-facing ports. Whichever side a move
  names as source issues `memprotocol.ReadReq`s (receiving `memprotocol.DataReadyRsp`); the
  destination side issues `memprotocol.WriteReq`s (receiving `memprotocol.WriteDoneRsp`).
- **Control** — accepts the uniform `memcontrolprotocol` commands (Enable,
  Pause, Drain, Reset).
