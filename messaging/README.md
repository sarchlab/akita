# messaging — Messages, Ports, and Connections

Package `messaging` provides messages, ports, and connections for the Akita
simulation framework. It is the communication layer: components own ports,
ports buffer messages, and a connection moves messages from one port's outgoing
buffer to another port's incoming buffer.

## Key Concepts

- A **message** (`Msg`) is any value carrying a `*MsgMeta` with routing and
  identification metadata. Bare `MsgMeta` is the envelope, not a message — it
  belongs to no protocol.
- A **protocol** (`Protocol`) is a set of message types organized into
  **roles** (`Role`), named after the package that defines it: its import
  path. Defining a protocol with `DefineProtocol` registers every message type
  it carries with the checkpoint codec; components tag each port with the
  role(s) it speaks, `akita:"role=<protocol>.<role>"`, for example
  `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`.
  Protocols are **opt-in**: messages flow without one, and registration only
  matters when a checkpoint can capture the message.
- A **port** is owned by a component and holds an incoming and an outgoing
  buffer. Components `Send`/`RetrieveIncoming` on their side; connections
  `Deliver`/`RetrieveOutgoing` on theirs.
- A **connection** is plugged into a set of ports and is responsible for
  delivering each outgoing message to the destination port named in its
  metadata.
- Ports are addressed by `RemotePort`, a string name. A message's `Src` and
  `Dst` are remote port names, not pointers.

## Key Types

### Msg and MsgMeta

```go
type Msg interface {
    Meta() *MsgMeta
}

type MsgMeta struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64 // ID of the request this responds to, if any
    SendTaskID   uint64
    RecvTaskID   uint64
}
```

Embed `MsgMeta` (or hold one) so a message satisfies `Msg`. `meta.IsRsp()`
reports whether `RspTo` is set.

**Checkpointing:** a message buffered in a port is serialized when the simulation
is checkpointed, so each concrete message type must be registered. The
recommended way is to declare the package's protocol once:

```go
var (
    Protocol  = messaging.DefineProtocol( // named ".../mem/memprotocol"
        messaging.RoleDef{Name: "requester",
            Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
        messaging.RoleDef{Name: "responder",
            Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
    )
    Requester = Protocol.Role("requester")
    Responder = Protocol.Role("responder")
)
```

and bind ports to roles with a tag on the component's Ports field:

```go
Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
```

The inspector checks each tag against the protocol's roles. Each protocol lives in its
own package (e.g. `mem/memprotocol`, `mem/memcontrolprotocol`, `mem/vm/vmprotocol`)
that owns the message types and the protocol definition. A
registration-coverage audit
(`protocolaudit_test.go`) fails CI for any message type in the module that is
not registered. `RegisterMsg(MyReq{})` in an `init()` remains as the low-level
primitive (and `RegisterEvent` for events). No custom marshalling is needed.
See [`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md).

### Port

```go
type Port interface {
    naming.Named
    hooking.Hookable

    AsRemote() RemotePort

    // For the component side
    CanSend() bool
    Send(msg Msg)
    PeekIncoming() (Msg, bool)
    RetrieveIncoming() (Msg, bool)

    // For the connection side
    Deliver(msg Msg)
    PeekOutgoing() (Msg, bool)
    RetrieveOutgoing() (Msg, bool)
    NotifyAvailable()

    SetConnection(conn Connection)
    Owner() PortOwner
    SetOwner(owner PortOwner)
    NumIncoming() int
    NumOutgoing() int
}
```

Create a port with `NewPort`:

```go
port := messaging.NewPort("MyComp.Top", incomingCap, outgoingCap)
```

In assembly, the system builder creates each port with no component and passes
it to the component's `Build`, which binds the port to the component and
registers it with the simulation (and the monitor).

`Send` pushes onto the outgoing buffer — callers must check `CanSend` first;
sending into a full buffer panics — and, when the buffer transitions from
empty, notifies the connection via `NotifySend`. `Deliver` pushes onto the
incoming buffer (guarded by `CanDeliver` the same way) and notifies the owning
component via `NotifyRecv`.

Reads return `(nil, false)` when empty. `Peek` leaves the message queued;
`Retrieve` consumes it and retains the sender notification when a full buffer
gains space. A successful read returns `true`. Capacity and peek checks do not
reserve buffer space or messages.

### Connection

```go
type Connection interface {
    naming.Named
    hooking.Hookable

    PlugIn(port Port)
    Unplug(port Port)
    NotifyAvailable(port Port)
    NotifySend()
}
```

A connection moves messages from outgoing to incoming buffers. `directconnection`
is the simplest implementation.

### PortOwner

```go
type PortOwner interface {
    NotifyRecv(port Port)
    NotifyPortFree(port Port)
}
```

A port notifies its owner when a message arrives in an empty incoming buffer
and when it can send again. The owner is usually a component
(`modeling.Component`), but `messaging` asks only for these two methods. The
interface says nothing about which ports an owner has or how it reaches them.
A component defined by a component model (`modeling/ticking` and its
siblings) holds its ports in a typed `Ports` struct: the system builder creates
each port with `NewPort` and passes them all to `Build`, which binds and
registers them.

## How It Works

1. Setup code creates each port with `NewPort` and passes it to its component's
   `Build`.
2. A connection is plugged into the ports with `PlugIn`, and each port's
   connection is set with `SetConnection`.
3. To send, a component builds a message with `Src`/`Dst` remote port names and
   calls `port.Send(msg)`.
4. The connection picks up the message via `PeekOutgoing`/`RetrieveOutgoing`,
   resolves `Dst`, and calls the destination port's `Deliver`.
5. The destination component is notified by `NotifyRecv` and consumes the
   message with `PeekIncoming`/`RetrieveIncoming`.

## Hooks

Ports are `hooking.Hookable` and fire hooks (with the message as `HookCtx.Item`)
at `HookPosPortMsgSend`, `HookPosPortMsgRecvd`, `HookPosPortMsgRetrieveIncoming`,
and `HookPosPortMsgRetrieveOutgoing`, which tracing and instrumentation use to
observe traffic.
