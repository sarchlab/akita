# rob — Reorder Buffer

Package `rob` provides a reorder-buffer component for the Akita simulation
framework. It restores in-order responses on top of a downstream memory unit
that may complete requests out of order. The reorder buffer forwards each
request from its `Top` port to a single bottom unit through its `Bottom` port,
tracks the resulting transactions in FIFO order, and releases responses to the
`Top` port strictly in the order the requests arrived.

## How It Works

A single `middleware` advances the buffer each tick, running three stages up to
`NumReqPerCycle` times. The control port is serviced first so a flush can
quiesce the pipeline before any new traffic moves.

```
Top ──► topDown ──► Bottom ──► (bottom unit) ──► Bottom ──► parseBottom
                                                                │
                          FIFO transaction list ◄──────────────┘
                                  │
                              bottomUp ──► Top   (released in arrival order)
```

1. **topDown** — Peeks a `memprotocol.AccessReq` (a `memprotocol.ReadReq` or `memprotocol.WriteReq`) from
   `Top`, builds a fresh *shadow* request with a new ID, rewrites its `Dst` to
   the configured `BottomUnit`, sends it on `Bottom`, and appends a transaction
   to the FIFO list. Stalls when the list reaches `BufferSize`.
2. **parseBottom** — Matches each `memprotocol.DataReadyRsp`/`memprotocol.WriteDoneRsp` from
   `Bottom` to its transaction by `RspTo`, records the payload, and sets the
   transaction's `HasRsp` flag. Unmatched responses (e.g. left over after a
   flush) are dropped.
3. **bottomUp** — If the head-of-line transaction has its response, forwards a
   matching response to the original requester (`RspTo` set to the original
   request ID) and pops it. A completed transaction behind an incomplete head
   waits, which is what enforces in-order delivery.

## Key Types

```go
type Comp = ticking.Component[Spec, state, modeling.None, Ports, middlewares]
```

- **Spec** — immutable config: `Freq`, `BufferSize` (max in-flight
  transactions), `NumReqPerCycle`, and the `BottomUnit` remote port every shadow
  request targets.
- **State** — mutable runtime: the FIFO `Transactions` list and an `IsFlushing`
  flag. Each `transactionState` remembers the original request's ID and source,
  the shadow request's ID, whether it is a read, and the buffered response data.

The reorder buffer references no shared resources, so its Resources type is
`modeling.None` and the system builder does not call `WithResources`.

## Builder Pattern

The reorder buffer is a ticking component (`modeling/ticking`). The system
builder builds it from `rob.Definition`: configuration is supplied as a whole
through `WithSpec` (start from `Definition.DefaultSpec`), and the port instances
with `component.BindPort` after Build. The system builder creates each port with
`twowaybuffered.NewPort`, choosing its buffer sizes, and names it
`component.BindPort("Field", port)`, which assigns `<instance>.<field>` and registers the port.

```go
spec := rob.Definition.DefaultSpec
spec.BufferSize = 256
spec.BottomUnit = dramPort.AsRemote()

reorderBuffer := rob.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    Build("ROB")
reorderBuffer.BindPort("Top", twowaybuffered.NewPort(8, 8))
reorderBuffer.BindPort("Bottom", twowaybuffered.NewPort(8, 8))
reorderBuffer.BindPort("Control", twowaybuffered.NewPort(8, 8))

topPort := reorderBuffer.Ports.Top
```

### Builder Methods

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec`. Set `BottomUnit` to the downstream port. |
| `component.BindPort("Field", p)` | Bind each port after Build; its owner assigns the full name. |

## Ports

- **Top** — accepts `memprotocol.ReadReq` and `memprotocol.WriteReq`, returns
  `memprotocol.DataReadyRsp` and `memprotocol.WriteDoneRsp` in arrival order.
- **Bottom** — sends shadow `memprotocol.ReadReq`/`memprotocol.WriteReq` to the `BottomUnit` and
  receives `memprotocol.DataReadyRsp`/`memprotocol.WriteDoneRsp`.
- **Control** — accepts `memcontrolprotocol.Req` (`CmdFlush` drops in-flight
  transactions and quiesces the pipeline; `CmdEnable` drains stale port traffic
  and resumes), returns `memcontrolprotocol.Rsp`.
