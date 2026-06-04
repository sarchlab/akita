---
sidebar_position: 1
---

# Ports and Messages

*Create a Component* showed the minimal component — one component, no
ports, one middleware. A real simulation is a graph of components that
exchange messages. This section shows the full pattern: **ports** for
inter-component messages, **multiple middlewares** in one component, and
the per-package **builder** convention used throughout Akita.

The example is `examples/tickingping`, a two-component setup where Agent A
sends ping messages to Agent B and Agent B replies after a fake latency.
Both agents use the same component type. The source is in
`examples/tickingping/`.

## What You Will Learn

- How a component declares **ports** and how messages flow through them.
- What "ticking" means and how middleware advances state each cycle.
- How `directconnection` moves messages between components.

The component still follows the Spec + State + middleware + builder shape
from the previous section — we are just adding ports and a second
middleware on top.

## Spec and State

```go
type Spec struct {
    Freq              timing.Freq `json:"freq"`
    OutPortBufferSize int         `json:"out_port_buffer_size"`
}

type State struct {
    StartTimes          []uint64               `json:"start_times"`
    NextSeqID           int                    `json:"next_seq_id"`
    NumPingNeedToSend   int                    `json:"num_ping_need_to_send"`
    PingDst             messaging.RemotePort   `json:"ping_dst"`
    CurrentTransactions []pingTransactionState `json:"current_transactions"`
}
```

Same rules as before: Spec is immutable configuration (here a clock
frequency and an output-port buffer size), State is mutable runtime data
(counters, the destination to ping, and in-flight transactions). Both are
JSON-serializable.

As in the previous section, alias the component type once:

```go
type Comp = modeling.Component[Spec, State, modeling.None]
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

`messaging.MsgMeta` is embedded in every message and carries routing
metadata (source, destination, ID, response-to-ID). The `SeqID` field is
the payload you actually care about.

## Ports

A **port** is how a component sends and receives messages. A component
adds named ports to itself; other components never touch a port's internal
fields — they reach it by name and send messages to its **remote**
address. In tickingping each agent has a single `Out` port that both sends
pings and receives them.

Two operations matter on a port:

- **`Send`** pushes a message into the port's outgoing buffer. If the
  buffer is full it returns an error, and the caller tries again next
  cycle.
- **`PeekIncoming` / `RetrieveIncoming`** read messages that have arrived.
  `Peek` looks without consuming, so you can decide you cannot handle a
  message yet and leave it; `Retrieve` is the commit.

The next page shows the middlewares that call these, and the builder that
creates the port.

## Where to Next

Next: the **builder** that creates the port internally and the two
middlewares that push and pull messages through it.
