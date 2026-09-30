---
sidebar_position: 1
---

# Ports and Messages

*Create a Component* showed the minimal component — one component, no
ports, one middleware. A real simulation is a graph of components that
exchange messages. This section shows the full pattern: **ports** for
inter-component messages, **multiple middlewares** in one component, and a
component that lives in **its own package**, declared by a `Definition` and
built by a system builder.

The example is `examples/tickingping`, a two-component setup where Agent A
sends ping messages to Agent B and Agent B replies after a fake latency.
Both agents use the same component type. The source is in
`examples/tickingping/`.

## What You Will Learn

- How a component declares **ports** and how messages flow through them.
- What "ticking" means and how middlewares advance state each cycle.
- How the system builder creates ports, and how `directconnection` moves
  messages between components.

The component still follows the five-struct shape from the previous section
— we are just adding a port and a second middleware on top.

## Spec and State

```go
type Spec struct {
    Freq timing.Freq `json:"freq"`

    // PingDst is the port the component sends its pings to.
    PingDst messaging.RemotePort `json:"ping_dst"`

    // NumPings is the number of pings the component sends, one per cycle
    // from its first tick. Zero makes a component that only responds.
    NumPings int `json:"num_pings"`
}

type State struct {
    StartTimes          []uint64               `json:"start_times"`
    NextSeqID           int                    `json:"next_seq_id"`
    CurrentTransactions []pingTransactionState `json:"current_transactions"`
}
```

Same rules as before: Spec is configuration, State is mutable runtime data
(the send time of each ping, the next sequence number, and the pings being
answered). Whom to ping and how many times are configuration, so they are
Spec fields; `PingDst` is a `messaging.RemotePort`, a port's name, so it is
still a scalar. Both structs are JSON-serializable. The component references
no shared objects, so its Resources is `modeling.None`.

As in the previous section, alias the component type once:

```go
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]
```

## Messages

Two message types, defined alongside the component:

```go
type pingReq struct {
    messaging.MsgMeta
    SeqID int
}

type pingRsp struct {
    messaging.MsgMeta
    SeqID int
}
```

Messages are **value types** — a `pingReq` is a plain struct, not a
pointer. You construct one with a composite literal (no `&`), send it by
value, and receive it by value.

`messaging.MsgMeta` is embedded in every message and carries routing
metadata. Its fields are `ID`, `Src`, `Dst`, `TrafficClass`,
`TrafficBytes`, and `RspTo`. Every message satisfies `messaging.Msg`,
whose `Meta() MsgMeta` method returns that metadata by value. The `SeqID`
field is the payload you actually care about.

## Ports

A **port** is how a component sends and receives messages. A component
lists its ports as the fields of its `Ports` struct, one `messaging.Port`
field per port:

```go
type Ports struct {
    // Out sends pings and responses and receives them from the peer.
    Out messaging.Port
}
```

Middlewares reach a port through its field, `m.comp.Ports.Out`. Other
components never touch the port directly — they send messages to its
**remote** address, `AsRemote()`. In tickingping each agent has a single
`Out` port that both sends pings and receives them.

The component only declares its ports; it never creates them. The system
builder creates each port, choosing its buffer sizes, and passes all of
them to `Build`, as the last page of this section shows. (A component with
a variable number of like ports, such as one per core, declares a port
group instead: a `[]messaging.Port` field. A field can also carry an
`akita:"role=<protocol>/<role>"` tag naming the protocol role the port
speaks; see *Protocols*. The examples' ports need neither.)

Two operations matter on a port:

- **`CanSend` / `Send`** push a message into the port's outgoing buffer.
  Check `CanSend()` first; if it returns true, `Send(msg)` enqueues the
  message (by value) and returns nothing. If `CanSend()` is false the
  buffer is full, so you leave the work and try again later.
- **`PeekIncoming` / `RetrieveIncoming`** read messages that have arrived.
  `Peek` looks without consuming, so you can decide you cannot handle a
  message yet and leave it; `Retrieve` is the commit.

The next page shows the middlewares that call these, and the `Definition`
that ties the component together.

## Where to Next

Next: the component's **Definition** and the two middlewares that push and
pull messages through its port.
