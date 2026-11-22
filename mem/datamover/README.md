# datamover — Streaming Data Mover

Package `datamover` provides a DMA-style data-mover component for the Akita
simulation framework. It copies a contiguous region of memory from a source to a
destination by issuing `memprotocol.ReadReq`s to one side, buffering the returned data,
and issuing `memprotocol.WriteReq`s to the other side. The mover owns no storage of its
own — it streams data between external memory controllers reachable through its
two memory-facing ports.

## How It Works

A move is driven by a single `Req` on the `Top` port. The
component admits up to `MaxTransactions` moves in arrival order:

```
Top ──► Req ──► dataTransferMW ──► Rsp ──► Top
                                      │
              ┌───────────────────────┴────────────────────────┐
        read side (src)                                  write side (dst)
   ReadReq ──► [Inside|Outside] ──► DataReadyRsp ──► buffer ──► WriteReq ──► [Inside|Outside] ──► WriteDoneRsp
```

The source and destination of each move can be `inside` or `outside`,
including same-side copies. Consecutive requests with the same source and
destination sides can overlap. A direction change waits at the head of the Top
queue until all active moves complete. Callers must order dependent copies by
waiting for the earlier response before submitting the later request.

Each tick the mover can issue one source read and one destination write and
consume one data response and one write acknowledgment. It issues reads in
admission order and writes from the oldest move with a ready chunk. Each copy
has its own sliding buffer; all copies share the read window set by `BufferSize`.
Source units occupy the window from read issue until their last requested byte
is sent to the destination, including over-fetched bytes. A read may start when
its first byte fits. For a 65-byte window and 64-byte source granularity, two
reads can be outstanding. The oldest copy that still has destination writes to
issue may exceed the window only for the source units its next write needs, so
an unaligned copy cannot stall on a small window. A copy with all writes issued
and only acknowledgments pending no longer gates this exception, so a later copy
may use it.

Source and destination addresses may have any byte offset. Every memory request
is an aligned request of that side's full granularity. Reads discard bytes
outside the requested range. A destination unit that is only partly covered,
including an aligned copy's short tail, is written as a full unit with
`DirtyMask` marking exactly the requested bytes; a fully covered unit has no
mask. The destination memory must honor `DirtyMask`.

A partial write is a full-unit read-merge-write in the destination memory, and
that merge is not atomic across components. While a partial write is in flight,
every access to that aligned destination unit must go through the same memory
controller or be otherwise serialized. With controllers that share one
`mem.Storage` and interleave more finely than the unit, or with parallel direct
storage writes to that unit, a neighboring update can be lost.

Each attached memory must accept a request of that side's full granularity. For
`mem/dram`, `TransactionQueueSize` must exceed the number of bursts touched by
one mover request, or the controller panics. If it does not, reduce the mover
granularity or increase the queue size.

The full aligned units touched on both sides must fit in backing storage, even
when the requested bytes end earlier. A unit that extends past capacity panics
in `mem.Storage`, on the source read or on the destination's masked merge, and
the serial engine returns that failure from `Run`.

Source and destination ranges that share backing memory must be disjoint,
because the mover streams forward and an overlapping copy can overwrite bytes it
has not read yet while still reporting completion. For example, with 64 B units
and `BufferSize` 1, a 192-byte copy from address 1 to address 130 is invalid.

Initialization rejects an active `InterleavingSize`, `RangeLow`, or `RangeHigh`
that is not a multiple of that side's granularity, because requests are routed
by their aligned unit base. Responses return on Top in
admission order after each copy's writes have been acknowledged.

## Key Types

```go
type Comp = ticking.Component[Spec, state, Resources, Ports, middlewares]

type Req struct {
    SrcAddress, DstAddress uint64
    ByteSize               uint64
    SrcSide, DstSide       Side // "inside" or "outside"
}

type Rsp struct {
}
```

- **Spec** — immutable config: `Freq`, `MaxTransactions`, `BufferSize`, and
  `InsideByteGranularity`/`OutsideByteGranularity`.
  `MaxTransactions` must be positive. `DefaultSpec()` sets it to 1 for serial
  operation. Existing Spec literals must set this field explicitly.
- **State** — accepted `Transactions` in order, each with its pending reads,
  pending writes, next addresses, and buffered data.
- **Resources** — empty. `Spec.Inside` and `Spec.Outside` are
  `mem.AddressMapper` values. Middleware routes directly through these values.
- **Ports** — the `Top`, `Inside`, `Outside`, and `Control` ports.
- **Middlewares** — `Ctrl` (control commands), `CtrlParse` (admits moves and
  completes finished ones), and `DataTransfer` (reads and writes), run in that
  order every cycle.
- **Comp** — `ticking.Component[Spec, state, Resources, Ports, middlewares]`, a
  ticking component.

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`DefaultSpec()`); the engine and registration come from
`WithSimulation`; side destinations and mapping parameters come from Spec; port
instances come from `component.BindPort` after Build.

```go
spec := datamover.DefaultSpec()
spec.BufferSize = 4096
spec.InsideByteGranularity = 64
spec.OutsideByteGranularity = 64
spec.Inside.Destinations = []messaging.RemotePort{"L2.Top"}
spec.Outside.Destinations = []messaging.RemotePort{"DRAM.Top"}

mover := modeling.MakeBuilder[*datamover.Comp](spec).
    WithSimulation(sim).
    Build("DMA")
mover.BindPort("Top", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
mover.BindPort("Inside", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
mover.BindPort("Outside", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())
mover.BindPort("Control", twowaybuffered.MakeBuilder().
    WithSimulation(sim).
    WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
    Build())

ctrlPort := mover.Ports.Control
```

### Builder Methods

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()`. |
| `WithResources(r)` | Optional empty Resources. |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

The system builder creates each port with `twowaybuffered.MakeBuilder`, choosing its
buffer sizes, and calls `component.BindPort("Field", port)` after Build.
After all connections are bound, `simulation.Initialize()` creates State and
middlewares and freezes the topology.

- **Top** — accepts `Req`, returns `Rsp` to the
  requester once the move completes.
- **Inside** / **Outside** — the two memory-facing ports. Whichever side a move
  names as source issues `memprotocol.ReadReq`s (receiving `memprotocol.DataReadyRsp`); the
  destination side issues `memprotocol.WriteReq`s (receiving `memprotocol.WriteDoneRsp`).
- **Control** — accepts the uniform `memcontrolprotocol` commands (Enable,
  Pause, Drain, Reset).
